import os
import sys

# Discos físicos mapeados no fstab (bases do MergerFS)
BRANCHES = [
    '/mnt/storage_blue',
    '/mnt/storage_wdblue',
    '/mnt/storage_samsung'
]

def format_size(bytes_size):
    if bytes_size >= 1073741824:
        return f"{bytes_size / 1073741824:.2f} GB"
    elif bytes_size >= 1048576:
        return f"{bytes_size / 1048576:.2f} MB"
    elif bytes_size >= 1024:
        return f"{bytes_size / 1024:.2f} KB"
    return f"{bytes_size} B"

def main():
    print("=== ANÁLISE DE STORAGE (MERGERFS) ===")
    print("Iniciando varredura nos discos físicos...")
    
    # Dicionário: caminho relativo -> lista de { branch, size }
    # Ex: 'filmes/Avatar.mkv' -> [{'branch': '/mnt/storage_blue', 'size': 1500000}]
    files_map = {}
    
    for branch in BRANCHES:
        if not os.path.exists(branch):
            print(f"  [!] Aviso: Disco {branch} não encontrado ou não montado. Pulando...")
            continue
            
        print(f"  -> Varrendo: {branch} ...")
        
        for root, dirs, files in os.walk(branch):
            # Ignorar pastas escondidas de lixeira padrão para evitar lixo
            if '.Trash' in root or '.Recycle' in root or 'lost+found' in root:
                continue
                
            for filename in files:
                full_path = os.path.join(root, filename)
                
                # O caminho relativo vai ser a "identidade" do arquivo no MergerFS
                # Exemplo: Se root for /mnt/storage_blue/filmes, rel_path = filmes/filename.mkv
                rel_path = os.path.relpath(full_path, branch)
                
                try:
                    size = os.path.getsize(full_path)
                except OSError:
                    continue
                
                # Vamos focar na análise de arquivos consideráveis (> 1MB)
                # para ignorar metadados (.nfo, imagens pequenas) e acelerar a visão de espaço
                if size > 1024 * 1024:
                    if rel_path not in files_map:
                        files_map[rel_path] = []
                    
                    files_map[rel_path].append({
                        'branch': branch,
                        'size': size
                    })

    # Processamento dos Resultados
    duplicates = {}
    branch_exclusives = {b: 0 for b in BRANCHES}
    
    for rel_path, locations in files_map.items():
        if len(locations) > 1:
            duplicates[rel_path] = locations
        elif len(locations) == 1:
            branch = locations[0]['branch']
            if branch in branch_exclusives:
                branch_exclusives[branch] += 1

    print("\n" + "="*50)
    print(f"RESUMO GERAL")
    print("="*50)
    print(f"Total de arquivos únicos catalogados (>1MB): {len(files_map)}\n")
    
    print("--- Arquivos Exclusivos (Presentes em apenas 1 disco) ---")
    for b in BRANCHES:
        print(f"  {b}: {branch_exclusives.get(b, 0)} arquivos")
        
    print("\n--- Arquivos Duplicados (Presentes em múltiplos discos) ---")
    if not duplicates:
        print("  ✅ Excelente! Nenhuma duplicação física encontrada.")
        print("  Todos os seus arquivos estão distribuídos corretamente.")
    else:
        print(f"  ❌ Atenção: {len(duplicates)} arquivos possuem cópias em mais de um disco físico.")
        
        wasted_space = 0
        print("\nLista de Duplicações Físicas:")
        print("-" * 50)
        for rel_path, locations in duplicates.items():
            print(f"Arquivo: {rel_path}")
            
            # Se um arquivo tem 3 cópias, 2 delas são desperdício
            file_size = locations[0]['size']
            wasted_space += file_size * (len(locations) - 1)
            
            for loc in locations:
                print(f"  -> Em: {loc['branch']} ({format_size(loc['size'])})")
            print("")
            
        print("-" * 50)
        print(f"⚠️ Espaço TOTAL desperdiçado por duplicações: {format_size(wasted_space)}")
        print("\nPara resolver as duplicações, você pode apagar manualmente a cópia")
        print("do disco que desejar (deixando apenas uma) ou rodar o seu script de fix_hardlinks.")
        print("="*50)

if __name__ == "__main__":
    main()
