import os
try:
    s1 = os.stat("/mnt/storage/downloads/incompletos/The Shadow's Edge (2025).mkv")
    s2 = os.stat("/mnt/storage/filmes/The Shadow's Edge (2025)/The Shadow's Edge (2025).mkv")
    s3 = os.stat("/mnt/storage/downloads/radarr/The Shadow's Edge (2025).mkv")
    print(f"DL Incompleto - Inode: {s1.st_ino}, Dev: {s1.st_dev}")
    print(f"LIB Filme     - Inode: {s2.st_ino}, Dev: {s2.st_dev}")
    print(f"DL Radarr     - Inode: {s3.st_ino}, Dev: {s3.st_dev}")
except Exception as e:
    print(f"Error: {e}")
