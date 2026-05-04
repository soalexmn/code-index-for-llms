package runners

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/code-index-for-llms/code-index/pkg/types"
)

// CCERunner invokes the code-context-engine CLI ("cce") as a subprocess.
// If the "cce" binary is not in PATH, Setup returns ErrRunnerUnavailable and
// the harness skips this runner rather than failing the test.
type CCERunner struct {
	fixtureDir string
	cceBin     string
	counter    TokenCounter
}

// NewCCERunner creates a CCERunner with the given token counter.
func NewCCERunner(counter TokenCounter) *CCERunner {
	return &CCERunner{counter: counter}
}

func (r *CCERunner) Name() string { return "cce" }

// Setup verifies that "cce" is available and runs "cce index" on the fixture
// directory to build the CCE index.
func (r *CCERunner) Setup(fixtureDir string) error {
	bin, err := findCCE()
	if err != nil {
		return ErrRunnerUnavailable
	}
	r.cceBin = bin
	r.fixtureDir = fixtureDir

	cmd := exec.Command(r.cceBin, "index")
	cmd.Dir = fixtureDir
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cce index failed: %w\noutput: %s", err, out)
	}
	return nil
}

// resultLineRe matches lines like "    1. auth\jwt.py:19-41"
var resultLineRe = regexp.MustCompile(`^\s+\d+\.\s+(.+):(\d+)-(\d+)\s*$`)

// Search calls "cce search <query> --top-k <topK>", parses the text output,
// then reads the actual chunk content from the fixture files by line range.
func (r *CCERunner) Search(query string, topK int) ([]types.SearchResult, time.Duration, error) {
	start := time.Now()

	cmd := exec.Command(r.cceBin, "search", query, "--top-k", strconv.Itoa(topK))
	cmd.Dir = r.fixtureDir
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, time.Since(start), fmt.Errorf("cce search: %w (stderr: %s)", err, stderr.String())
	}

	latency := time.Since(start)
	results, err := parseCCEOutput(stdout.String(), r.fixtureDir)
	return results, latency, err
}

// parseCCEOutput converts cce's plain-text search output into SearchResults.
// Each result line has the form "    N. path\file.py:startLine-endLine".
// Content is read from disk using the reported line range.
func parseCCEOutput(output, fixtureDir string) ([]types.SearchResult, error) {
	var results []types.SearchResult
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		m := resultLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		rawPath := m[1]
		startLine, _ := strconv.Atoi(m[2])
		endLine, _ := strconv.Atoi(m[3])

		// Normalise to forward-slash relative path.
		relPath := filepath.ToSlash(strings.TrimSpace(rawPath))

		// Read the chunk content from disk.
		absPath := filepath.Join(fixtureDir, filepath.FromSlash(relPath))
		content := readLines(absPath, startLine, endLine)

		// Derive chunk name from the first non-blank content line.
		name := chunkName(relPath, content)

		results = append(results, types.SearchResult{
			Chunk: types.Chunk{
				FilePath:  relPath,
				Name:      name,
				Content:   content,
				StartLine: startLine,
				EndLine:   endLine,
			},
			Score: 1.0 / float64(len(results)+1), // rank-based pseudo-score
		})
	}
	return results, scanner.Err()
}

// readLines reads lines [start, end] (1-based, inclusive) from a file.
func readLines(absPath string, start, end int) string {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start > end {
		return ""
	}
	return strings.Join(lines[start-1:end], "\n")
}

// chunkName extracts a name from the first meaningful line of content,
// falling back to the file stem when nothing useful is found.
func chunkName(relPath, content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Extract identifier after def/func/class/type/resource/variable keywords.
		for _, prefix := range []string{"def ", "async def ", "func ", "class ", "type ", "resource ", "variable "} {
			if strings.HasPrefix(line, prefix) {
				rest := strings.TrimPrefix(line, prefix)
				// Take up to first '(' or '{' or ' '.
				if i := strings.IndexAny(rest, "({ "); i > 0 {
					return rest[:i]
				}
				return rest
			}
		}
		// Terraform resource blocks: resource "aws_s3_bucket" "logs" {
		if strings.HasPrefix(line, "resource \"") || strings.HasPrefix(line, "variable \"") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				rtype := strings.Trim(parts[1], "\"")
				rname := strings.Trim(parts[2], "\"")
				return rtype + "." + rname
			}
		}
		break
	}
	// Fallback: file stem.
	base := filepath.Base(relPath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// ContextTokens sums the token count of all retrieved chunk contents.
func (r *CCERunner) ContextTokens(results []types.SearchResult) int {
	total := 0
	for _, res := range results {
		total += r.counter.Count(res.Chunk.Content)
	}
	return total
}

// OutputTokensEstimate returns the baseline unchanged (CCE does not compress output).
func (r *CCERunner) OutputTokensEstimate(baselineOutputTokens int) int {
	return baselineOutputTokens
}

// findCCE locates the cce binary via PATH, then falls back to common Windows
// Python Scripts directories (which may not be in the inherited PATH when the
// process started before Python was installed).
func findCCE() (string, error) {
	if bin, err := exec.LookPath("cce"); err == nil {
		return bin, nil
	}
	// Windows fallback: check Python 3.x Scripts dirs under LOCALAPPDATA.
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		for _, ver := range []string{"Python311", "Python312", "Python313", "Python310"} {
			c := filepath.Join(local, "Programs", "Python", ver, "Scripts", "cce.exe")
			if _, err := os.Stat(c); err == nil {
				return c, nil
			}
		}
	}
	if profile := os.Getenv("USERPROFILE"); profile != "" {
		for _, ver := range []string{"Python311", "Python312", "Python313", "Python310"} {
			c := filepath.Join(profile, "AppData", "Local", "Programs", "Python", ver, "Scripts", "cce.exe")
			if _, err := os.Stat(c); err == nil {
				return c, nil
			}
		}
	}
	return "", fmt.Errorf("cce not found")
}

// Cleanup removes the CCE index directory from the fixture to avoid pollution.
func (r *CCERunner) Cleanup() error {
	if r.fixtureDir == "" {
		return nil
	}
	// CCE stores its index in .cce/ - remove it after the run.
	_ = os.RemoveAll(filepath.Join(r.fixtureDir, ".cce"))
	return nil
}
