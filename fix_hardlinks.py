import os
import subprocess

def format_size(bytes):
    if bytes >= 1073741824:
        return f"{bytes / 1073741824:.2f} GB"
    elif bytes >= 1048576:
        return f"{bytes / 1048576:.2f} MB"
    elif bytes >= 1024:
        return f"{bytes / 1024:.2f} KB"
    return f"{bytes} B"

def get_files(base_path):
    files = {}
    # Procuramos apenas pastas que pareçam ser HDs físicos (storage_*)
    # ou o proprio ponto de montagem se for o caso
    for root, _, filenames in os.walk(base_path):
        for filename in filenames:
            file_path = os.path.join(root, filename)
            try:
                stat = os.stat(file_path)
                size = stat.st_size
                inode = stat.st_ino
                dev = stat.st_dev
                if size > 50 * 1024 * 1024:  # Apenas arquivos > 50MB (mais rápido)
                    if size not in files:
                        files[size] = []
                    files[size].append({
                        'path': file_path, 
                        'inode': inode, 
                        'dev': dev,
                        'name': filename
                    })
            except OSError:
                continue
    return files

def process_hd(hd_path):
    print(f"\n📂 Analisando HD Físico: {hd_path}")
    downloads_path = os.path.join(hd_path, 'downloads')
    filmes_path = os.path.join(hd_path, 'filmes')
    series_path = os.path.join(hd_path, 'series')

    if not os.path.exists(downloads_path):
        return 0

    downloads = get_files(downloads_path)
    library = {}
    
    if os.path.exists(filmes_path):
        for size, fls in get_files(filmes_path).items():
            library[size] = library.get(size, []) + fls
    if os.path.exists(series_path):
        for size, fls in get_files(series_path).items():
            library[size] = library.get(size, []) + fls

    tasks = []
    total_saving = 0
    
    for size, dl_files in downloads.items():
        if size in library:
            lib_files = library[size]
            for dl in dl_files:
                for lib in lib_files:
                    if dl['inode'] == lib['inode']:
                        continue
                    
                    # Match por tamanho (quase impossível errar em arquivos grandes)
                    tasks.append({'dl': dl, 'lib': lib, 'size': size})
                    if not any(t['lib']['path'] == lib['path'] for t in tasks[:-1]):
                        total_saving += size
                        print(f"  [!] Encontrado: {lib['name']} ({format_size(size)})")

    if not tasks:
        print("  ✨ Tudo ok neste HD.")
        return 0

    print(f"  ⚠️  Encontrados {len(tasks)} arquivos duplicados neste HD.")
    confirm = input(f"  Deseja linkar estes arquivos no HD {os.path.basename(hd_path)}? (y/n): ")
    
    if confirm.lower() == 'y':
        for task in tasks:
            try:
                temp = task['lib']['path'] + ".tmp"
                os.link(task['dl']['path'], temp)
                os.replace(temp, task['lib']['path'])
                print(f"    [OK] {task['lib']['name']}")
            except Exception as e:
                print(f"    [ERRO] {task['lib']['name']}: {e}")
        return total_saving
    return 0

def main():
    print("🚀 Início da Otimização de Storage (Modo Físico)")
    
    # Lista de HDs físicos baseada no que vi no seu sistema
    hds = [
        '/mnt/storage_blue',
        '/mnt/storage_samsung',
        '/mnt/storage_wdblue'
    ]
    
    total_recovered = 0
    for hd in hds:
        if os.path.exists(hd):
            total_recovered += process_hd(hd)
        else:
            print(f"\n🚫 HD não encontrado: {hd}")

    print("\n" + "="*50)
    print(f"FINALIZADO! Total de espaço físico recuperado: {format_size(total_recovered)}")
    print("="*50)

if __name__ == "__main__":
    main()
