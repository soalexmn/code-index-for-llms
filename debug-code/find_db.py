import os
import glob

# Find all index.db files
for root in ["C:/", "C:/Users", "C:/Temp", "C:/Windows/Temp"]:
    for dirpath, dirnames, filenames in os.walk(root):
        if "index.db" in filenames and ".code-index" in dirpath:
            print(os.path.join(dirpath, "index.db"))
        # Avoid going too deep
        depth = dirpath.replace(root, "").count(os.sep)
        if depth > 6:
            dirnames.clear()
