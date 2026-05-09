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

def get_files(path):
    files = {}
    for root, _, filenames in os.walk(path):
        for filename in filenames:
            file_path = os.path.join(root, filename)
            try:
                stat = os.stat(file_path)
                size = stat.st_size
                inode = stat.st_ino
                if size > 1024 * 1024:  # Only files > 1MB
                    if size not in files:
                        files[size] = []
                    files[size].append({'path': file_path, 'inode': inode, 'name': filename})
            except OSError:
                continue
    return files

def main():
    print("Scanning storage...")
    downloads = get_files('/mnt/storage/downloads')
    library_filmes = get_files('/mnt/storage/filmes')
    library_series = get_files('/mnt/storage/series')
    
    all_library = {}
    for size, fls in library_filmes.items():
        all_library[size] = all_library.get(size, []) + fls
    for size, fls in library_series.items():
        all_library[size] = all_library.get(size, []) + fls

    print("Comparing files...")
    tasks = []
    total_saving = 0

    for size, dl_files in downloads.items():
        if size in all_library:
            lib_files = all_library[size]
            for dl in dl_files:
                for lib in lib_files:
                    if dl['inode'] != lib['inode']:
                        # Different inodes, potential copy
                        is_match = False
                        if size > 100 * 1024 * 1024:
                            is_match = True # Very unlikely to have two different 100MB+ files of exact same size
                        elif dl['name'].lower() in lib['name'].lower() or lib['name'].lower() in dl['name'].lower():
                            is_match = True
                        
                        if is_match:
                            tasks.append({'dl': dl, 'lib': lib, 'size': size})
                            total_saving += size
                            print(f"Encontrado: {dl['name']} <-> {lib['name']} ({format_size(size)})")

    if not tasks:
        print("Nenhuma cópia física encontrada. Tudo já parece estar linkado!")
        return

    print("\n-------------------------------------------------------")
    print("RESUMO DA OPERAÇÃO:")
    print(f"Arquivos a linkar: {len(tasks)}")
    print(f"Espaço a ser recuperado: {format_size(total_saving)}")
    print("-------------------------------------------------------")
    print("O processo vai deletar o arquivo na biblioteca e criar")
    print("um hardlink apontando para o arquivo em 'downloads'.\n")
    
    confirm = input("Deseja continuar? (y/n): ")
    
    if confirm.lower() == 'y':
        print("Iniciando processamento...")
        for task in tasks:
            dl = task['dl']
            lib = task['lib']
            
            print(f"  Fixing: ln -f '{dl['path']}' '{lib['path']}'")
            try:
                os.remove(lib['path'])
                subprocess.run(['ln', dl['path'], lib['path']], check=True)
                print("  [OK] Linkado!")
            except Exception as e:
                print(f"  [ERRO] Falha ao corrigir: {e}")
                
        print("-------------------------------------------------------")
        print("Sucesso! O espaço foi liberado fisicamente no disco.")
    else:
        print("Operação cancelada pelo usuário.")

if __name__ == "__main__":
    main()
