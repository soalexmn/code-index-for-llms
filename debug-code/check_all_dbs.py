import sqlite3

dbs = [
    "C:/tmp/pytest/.code-index/index.db",
    "C:/Users/masso/AppData/Local/Temp/pytest/.code-index/index.db",
]

for p in dbs:
    try:
        conn = sqlite3.connect(p)
        tables = [r[0] for r in conn.execute("SELECT name FROM sqlite_master WHERE type='table'").fetchall()]
        print(f"\n{p}")
        print("Tables:", tables)
        if "projects" in tables:
            rows = conn.execute("SELECT id, root_path, name FROM projects").fetchall()
            print("Projects:", rows)
        conn.close()
    except Exception as e:
        print(f"{p}: {e}")
