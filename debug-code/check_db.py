import sqlite3
import os
import glob

# Find the pytest index DB
search_paths = [
    "/tmp/pytest/.code-index/index.db",
    "C:/Users/masso/AppData/Local/Temp/pytest/.code-index/index.db",
]

for p in search_paths:
    if os.path.exists(p):
        print(f"Found DB at: {p}")
        conn = sqlite3.connect(p)
        try:
            rows = conn.execute("SELECT * FROM projects").fetchall()
            print("Projects:", rows)
            chunks = conn.execute("SELECT COUNT(*) FROM chunks").fetchone()
            print("Chunks:", chunks)
        except Exception as e:
            print("Error:", e)
        finally:
            conn.close()
        break
else:
    print("DB not found at expected locations")
    print("Checking /tmp:", os.listdir("/tmp") if os.path.exists("/tmp") else "no /tmp")
