package terraform

import (
	"testing"

	"github.com/code-index-for-llms/code-index/pkg/types"
)

var tfFixture = `
terraform {
  required_version = ">= 1.0"
}

provider "aws" {
  region = "us-east-1"
}

resource "aws_s3_bucket" "main" {
  bucket = "my-bucket"
  tags = {
    Environment = "prod"
  }
}

resource "aws_s3_bucket_versioning" "main" {
  bucket = aws_s3_bucket.main.id
  versioning_configuration {
    status = "Enabled"
  }
}

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 3.0"
  cidr    = "10.0.0.0/16"
}

variable "region" {
  type    = string
  default = "us-east-1"
}

output "bucket_arn" {
  value = aws_s3_bucket.main.arn
}

data "aws_ami" "ubuntu" {
  most_recent = true
}
`

func TestParse_BlockTypes(t *testing.T) {
	p := New()
	chunks, err := p.Parse("main.tf", []byte(tfFixture))
	if err != nil {
		t.Fatal(err)
	}

	byName := map[string]types.Chunk{}
	for _, c := range chunks {
		byName[c.Name] = c
	}

	cases := []struct {
		name     string
		wantType types.ChunkType
	}{
		{"aws_s3_bucket.main", types.ChunkTypeResource},
		{"aws_s3_bucket_versioning.main", types.ChunkTypeResource},
		{"module.vpc", types.ChunkTypeModule},
		{"var.region", types.ChunkTypeVariable},
		{"output.bucket_arn", types.ChunkTypeVariable},
		{"data.aws_ami.ubuntu", types.ChunkTypeResource},
	}

	for _, tc := range cases {
		c, ok := byName[tc.name]
		if !ok {
			t.Errorf("missing chunk %q (have: %v)", tc.name, chunkNames(chunks))
			continue
		}
		if c.ChunkType != tc.wantType {
			t.Errorf("%q.ChunkType = %q, want %q", tc.name, c.ChunkType, tc.wantType)
		}
	}
}

func TestParse_ResourceMetadata(t *testing.T) {
	p := New()
	chunks, err := p.Parse("main.tf", []byte(tfFixture))
	if err != nil {
		t.Fatal(err)
	}

	var s3 *types.Chunk
	for i := range chunks {
		if chunks[i].Name == "aws_s3_bucket.main" {
			s3 = &chunks[i]
			break
		}
	}
	if s3 == nil {
		t.Fatal("aws_s3_bucket.main not found")
	}

	want := map[string]string{
		"resource_type": "aws_s3_bucket",
		"resource_name": "main",
		"provider":      "aws",
	}
	for k, v := range want {
		if got := s3.Metadata[k]; got != v {
			t.Errorf("metadata[%q] = %q, want %q", k, got, v)
		}
	}
}

func TestParse_DataSource(t *testing.T) {
	p := New()
	chunks, err := p.Parse("main.tf", []byte(tfFixture))
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range chunks {
		if c.Name == "data.aws_ami.ubuntu" {
			if c.Metadata["is_data_source"] != "true" {
				t.Errorf("data source missing is_data_source=true metadata")
			}
			return
		}
	}
	t.Error("data.aws_ami.ubuntu not found")
}

func TestParse_FilePath(t *testing.T) {
	p := New()
	chunks, err := p.Parse("infra/main.tf", []byte(tfFixture))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range chunks {
		if c.FilePath != "infra/main.tf" {
			t.Errorf("FilePath = %q, want %q", c.FilePath, "infra/main.tf")
		}
	}
}

func TestParse_LineNumbers(t *testing.T) {
	src := `resource "aws_s3_bucket" "logs" {
  bucket = "my-logs"
}

module "networking" {
  source = "./modules/vpc"
}
`
	p := New()
	chunks, err := p.Parse("t.tf", []byte(src))
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range chunks {
		switch c.Name {
		case "aws_s3_bucket.logs":
			if c.StartLine != 1 {
				t.Errorf("aws_s3_bucket.logs.StartLine = %d, want 1", c.StartLine)
			}
		case "module.networking":
			if c.StartLine != 5 {
				t.Errorf("module.networking.StartLine = %d, want 5", c.StartLine)
			}
		}
	}
}

func TestExtractSymbols(t *testing.T) {
	src := `resource "aws_s3_bucket" "main" { bucket = "b" }
variable "name" { type = string }
`
	p := New()
	chunks, _ := p.Parse("t.tf", []byte(src))
	syms, err := p.ExtractSymbols(chunks)
	if err != nil {
		t.Fatal(err)
	}
	if len(syms) == 0 {
		t.Error("want at least 1 symbol, got 0")
	}
}

func chunkNames(chunks []types.Chunk) []string {
	names := make([]string, len(chunks))
	for i, c := range chunks {
		names[i] = string(c.ChunkType) + ":" + c.Name
	}
	return names
}
