# 🚀 Rockchip Home Server Dashboard

Este projeto gerencia um servidor doméstico baseado em **Rockchip (Rock 3A)**, integrando monitoramento de hardware, automação de downloads e gerenciamento de mídia.

## 🏗️ Estrutura do Projeto

*   **`node-rockchip`**: Dashboard central e API de monitoramento de hardware (CPU, GPU, Discos, Temperaturas).
*   **`bot-rockchip`**: TeleBot para gerenciamento e download de vídeos e arquivos via Telegram.
*   **`docker-compose.yml`**: Stack de automação de mídia (Sonarr, Radarr, Bazarr, Prowlarr, Telegram API).

---

## 🛠️ Comandos Essenciais

### 🔑 Login sem Senha (SSH Keys)
Para fazer deploy sem precisar digitar a senha toda hora, configure uma chave SSH:

1.  **Gere a chave no seu PC** (se ainda não tiver):
    ```powershell
    # No PowerShell ou CMD:
    ssh-keygen -t ed25519
    ```
2.  **Envie a chave para o servidor**:
    ```powershell
    # No PowerShell (substitua user@host):
    cat $HOME\.ssh\id_ed25519.pub | ssh user@host "mkdir -p ~/.ssh && cat >> ~/.ssh/authorized_keys"
    ```
    *(Após isso, o `ssh` e o `scp` não pedirão mais senha)*

3.  **Dica (Atalho no Windows)**:
    Crie o arquivo `$HOME\.ssh\config` com:
    ```text
    Host rock
        HostName 192.168.1.110
        User olucas
    ```
    Isso permite usar apenas `ssh rock` ou `scp arquivo rock:~/`.

### 🔐 Sudo sem Senha (Deploy de Serviços)
Para permitir que o script de deploy reinicie serviços (`systemctl`) sem pedir a senha do `sudo`:

1.  **Crie a regra para o serviço específico** (ex: `torrent-sniffer`):
    ```bash
    echo "olucas ALL=(ALL) NOPASSWD: /bin/systemctl start torrent-sniffer, /bin/systemctl stop torrent-sniffer, /bin/systemctl restart torrent-sniffer, /bin/systemctl status torrent-sniffer" | sudo tee /etc/sudoers.d/torrent-sniffer
    ```

2.  **Para verificar as permissões atuais**:
    ```bash
    sudo -l
    ```

### 🚀 Transferindo Arquivos (PC → Server)
Para enviar arquivos do seu computador para o Rockchip via rede:
```bash
# Copiar o docker-compose.yml para a pasta home
scp ./docker-compose.yml user@host:~/

# Copiar uma pasta inteira (recursivamente)
scp -r ./node-rockchip user@host:~/

# Copiar um binário compilado
scp ./rockchip-node-linux-arm64 user@host:~/
```

### 📂 Movendo o Docker Compose dentro do Servidor
Geralmente a stack de mídia fica em uma pasta específica. Para mover o arquivo para o local correto no servidor:
```bash
# Cria a pasta se não existir
mkdir -p ~/arr-stack

# Move o arquivo
mv ~/docker-compose.yml ~/arr-stack/docker-compose.yml
```

### 🔐 Permissões e Execução
Se os binários não estiverem executando, garanta que eles tenham permissão X:
```bash
chmod +x ~/rockchip-node-linux-arm64
chmod +x ~/telebot-linux-arm64
```

### 🚢 Gerenciando Docker Compose
```bash
cd ~/arr-stack
docker compose up -d      # Inicia tudo em background
docker compose restart    # Reinicia todos os containers
docker compose logs -f    # Acompanha logs em tempo real
```

---

## 🔧 Instalando um Novo Serviço (Systemd)

Para transformar qualquer programa em um serviço que inicia com o sistema:

1.  **Crie o arquivo do serviço**:
    Crie um arquivo em `/etc/systemd/system/meu-servico.service`:
    ```ini
    [Unit]
    Description=Descrição do meu serviço
    After=network.target

    [Service]
    Type=simple
    User=olucas
    WorkingDirectory=/home/olucas
    ExecStart=/home/olucas/meu-programa
    Restart=always

    [Install]
    WantedBy=multi-user.target
    ```

2.  **Habilite e Inicie**:
    ```bash
    sudo systemctl daemon-reload            # Atualiza o systemd
    sudo systemctl enable meu-servico       # Inicia no boot
    sudo systemctl start meu-servico        # Inicia agora
    ```

3.  **Verifique o Status**:
    ```bash
    sudo systemctl status meu-servico
    journalctl -u meu-servico -f            # Ver logs em tempo real
    ```

---

## 🚀 Deploy Automático
Para atualizar o servidor a partir da sua máquina local:
```powershell
./deploy-all.sh
```
Isso compilará o código para ARM64, enviará via SCP e reiniciará os serviços no Rockchip.

---

## 📦 Verificando Hard Links e Inodes

Para garantir que o Radarr/Sonarr estão economizando espaço usando Hard Links (um único arquivo físico apontado por dois nomes diferentes), use os comandos abaixo:

### 🔍 Ver o ID (Inode) de um arquivo específico:
```bash
ls -i "/mnt/storage_blue/filmes/Filme (2024)/filme.mkv"
# Saída exemplo: 12345678 filme.mkv (O número é o ID único do arquivo no disco)
```

### 📂 Localizar todas as pastas onde esse arquivo se encontra:
Se dois arquivos têm o mesmo Inode, eles são o **mesmo arquivo físico**. Para achar as cópias ligadas:
```bash
# Substitua o número pelo Inode encontrado no comando anterior
find /mnt/storage_blue -inum 12345678
```

### 📋 Listar todos os Hard Links do disco:
Este comando mostra todos os arquivos que possuem mais de 1 link (estão em mais de uma pasta ao mesmo tempo):
```bash
find /mnt/storage_blue -type f -links +1 -printf "%i %p\n" | sort -n
```

---

## 🌐 Acesso
*   **Dashboard**: `http://host:5000`
*   **TeleBot API**: `http://host:5001`
