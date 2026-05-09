# Tutorial: Formatando e Adicionando um Novo HD ao MergerFS

Este guia passo a passo vai te ajudar a inicializar um HD novo, formatá-lo e integrá-lo ao seu pool existente do MergerFS no Linux.

> **⚠️ AVISO IMPORTANTE:** Tenha **muito cuidado** ao executar os comandos de formatação e particionamento (passos 1 e 2). Identifique o disco correto para não apagar acidentalmente os dados de outros HDs que já possuam arquivos!

---

## Passo 1: Identificar o novo HD
Primeiro, você precisa descobrir qual é o caminho (device path) do seu novo HD no sistema.

Execute o comando abaixo:
```bash
lsblk
```
A saída mostrará todos os discos conectados. Procure pelo disco novo (ele geralmente não terá partições e o tamanho baterá com a capacidade do disco recém-comprado). 

Vamos assumir para este tutorial que o disco novo seja o **`/dev/sdX`**. *(Substitua o `X` pela letra correspondente ao seu disco real)*.

---

## Passo 2: Particionar e Formatar o HD
Vamos criar uma tabela de partições e formatar o disco em `ext4` (um dos formatos mais recomendados para discos de armazenamento no Linux).

1. Crie uma tabela de partição (GPT) e uma partição primária usando o `parted`:
```bash
sudo parted /dev/sdX mklabel gpt
sudo parted -a opt /dev/sdX mkpart primary ext4 0% 100%
```
*(Isso criará uma partição cobrindo todo o disco, que será chamada de `/dev/sdX1`)*.

2. Formate a partição recém-criada para `ext4`:
```bash
sudo mkfs.ext4 -L disco_novo /dev/sdX1
```

---

## Passo 3: Criar o Ponto de Montagem
Você precisa de um diretório vazio para que o Linux monte o disco de forma permanente.
Vamos assumir que você vai chamá-lo de `disk3` (aju ste o número de acordo com quantos discos você já possui).

```bash
sudo mkdir -p /mnt/disk3
```

---

## Passo 4: Descobrir o UUID do novo HD
Para garantir que o disco monte corretamente sempre que o servidor reiniciar, usaremos o UUID (Identificador Único) da partição.

```bash
sudo blkid /dev/sdX1
```
Anote o UUID retornado (ele parecerá algo como `UUID="1234abcd-12ab-34cd-56ef-1234567890ab"`). Você vai precisar dele no próximo passo.

---

## Passo 5: Montar o Disco Permanentemente
Agora vamos adicionar o disco ao `/etc/fstab` para montagem automática.

1. Abra o arquivo `/etc/fstab` no editor:
```bash
sudo nano /etc/fstab
```

2. Vá até o final do arquivo e adicione a linha do seu novo disco (usando o UUID do Passo 4 sem as aspas, e o seu ponto de montagem):
```text
UUID=seu-uuid-aqui /mnt/disk3 ext4 defaults 0 2
```

3. Salve o arquivo e saia do editor (No Nano: `Ctrl+O`, `Enter`, `Ctrl+X`).

4. Teste se as configurações do `fstab` estão corretas montando tudo:
```bash
sudo mount -a
```

5. Verifique se o disco foi montado no caminho esperado:
```bash
df -h
```

---

## Passo 6: Adicionar ao MergerFS
Seu disco está montado! Agora, você precisa atualizar a configuração do MergerFS. Geralmente, o MergerFS também é montado via `/etc/fstab`.

1. Abra novamente o arquivo:
```bash
sudo nano /etc/fstab
```

2. Encontre a linha que configura o seu MergerFS. Ela deve ser parecida com isso:
```text
/mnt/disk1:/mnt/disk2 /mnt/storage fuse.mergerfs defaults,allow_other,use_ino,... 0 0
```

3. Adicione o seu novo ponto de montagem (ex: `/mnt/disk3`). 
   - **Opção A (Padrão de nome - Globbing):** Se os seus discos seguem um padrão, como `/mnt/disk1`, `/mnt/disk2`, `/mnt/disk3`, e a linha do MergerFS estiver configurada com `/mnt/disk*`, **você não precisa alterar a linha**! O MergerFS já pegará o novo disco automaticamente ao ser remontado.
   - **Opção B (Caminhos explícitos):** Se os caminhos estiverem separados por `:`, adicione o novo disco na lista.
     **Exemplo:**
     ```text
     /mnt/disk1:/mnt/disk2:/mnt/disk3 /mnt/storage fuse.mergerfs defaults,allow_other,use_ino 0 0
     ```

4. Salve e saia do editor.

5. Remonte o pool do MergerFS:
```bash
sudo umount /mnt/storage
sudo mount -a
```

---

## Passo 7: Ajustar Permissões (Opcional, porém recomendado)
Logo após a formatação, o HD pertencerá ao usuário `root`. Dependendo das suas aplicações (como Docker ou Samba), é melhor garantir que o seu usuário tenha controle sobre o disco.

```bash
sudo chown -R $USER:$USER /mnt/disk3
```

**Pronto!** Seu novo HD está formatado e operando no seu pool do MergerFS.
