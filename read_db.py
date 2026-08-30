import sqlite3
import os

db_path = r'c:\Users\olucas\Documents\homelab\torrent-sniffer\sniffer.db'
if not os.path.exists(db_path):
    print(f"Database not found at {db_path}")
    exit(1)

conn = sqlite3.connect(db_path)
cursor = conn.cursor()
cursor.execute("SELECT name, file_name, file_size FROM sniffs ORDER BY id DESC LIMIT 10")
rows = cursor.fetchall()
for row in rows:
    print(f"Name: {row[0]}")
    print(f"File: {row[1]}")
    print(f"Size: {row[2] / 1024 / 1024:.2f} MB")
    print("-" * 20)
conn.close()
