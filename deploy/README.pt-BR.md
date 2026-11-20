# Instalar o Stuarty em um servidor

Este guia está em português por pedido do plano (épico 25); o texto de referência, em inglês, é a
página "Install on a server" da documentação (`site/content/docs/en/self-hosting/vps.mdx`) e a
especificação é a seção 21 do SDD-009.

## O que você vai aprender

- como rodar o Stuarty em um servidor Linux alugado, no seu domínio, com HTTPS;
- como atualizar, e o que acontece quando a versão nova não sobe;
- como fazer backup, restaurar em um servidor novo e por que a chave mestra fica separada.

## A ideia

O pacote é a pasta `deploy/` do repositório: um arquivo Docker Compose com dois contêineres, o Stuarty e o
[Caddy](https://caddyserver.com/), e quatro scripts. O Caddy usa as portas 80 e 443, obtém o certificado do
seu domínio e repassa as requisições ao Stuarty. O Stuarty escuta só na rede interna, roda sem ser root, com o
sistema de arquivos da raiz somente leitura e sem capacidades extras, e guarda tudo em um volume.

Três segredos ficam em `deploy/secrets/`, como arquivos: a **chave mestra** (ela sela as credenciais das contas
que você conectou), a chave da aplicação e a senha do primeiro administrador. Eles são criados uma vez e nunca
substituídos. Nenhum está na imagem, em `deploy/.env` ou em um backup.

A imagem é construída no seu servidor a partir do código da versão que você baixou. Nada é baixado de um
registro do Stuarty.

A que o pacote se conecta:

- O Caddy pede o certificado do seu domínio a uma autoridade certificadora e o renova. É a única
  conexão de saída que o próprio pacote precisa.
- Construir a imagem baixa as imagens base, os pacotes Go e JavaScript e o FFmpeg fixado. Acontece na
  instalação e na atualização, no servidor.
- `update.sh` consulta o repositório git de onde você clonou para ver as tags novas.
- O Stuarty em si não abre nenhuma conexão de saída até você conectar uma conta ou um provedor.

## Começo rápido

Você precisa de um servidor Linux com Docker 24 ou mais novo e o plugin Compose 2.20 ou mais novo, de um domínio
cujo registro DNS aponte para o servidor e das portas 80 e 443 abertas.

Escolha a tag de uma versão e instale:

```console
git clone --branch vX.Y.Z https://github.com/AlexGalhardo/stuarty.git
cd stuarty
bash deploy/install.sh --domain stuarty.example.com --email voce@example.com
```

O script confere o Docker, cria as chaves, grava `deploy/.env` (modo 0600), constrói a imagem, sobe os
contêineres, espera até que estejam saudáveis e até que `https://stuarty.example.com/up` responda, e mostra o
endereço, o e-mail e a primeira senha. A primeira construção leva vários minutos. Entre e troque a senha.

Pode rodar de novo quando quiser: ele não muda nenhum arquivo, mantém todas as chaves e mostra onde está a
senha, não a senha.

### Atualizar

```console
bash deploy/update.sh X.Y.Z
```

Ele faz um backup antes, troca para a tag `vX.Y.Z`, constrói a imagem, inicia (as migrações rodam na partida)
e espera ficar saudável. Se não ficar saudável em dois minutos, ele para a versão nova, volta o código para a
tag anterior, restaura o backup e sobe a imagem anterior. O que a versão que falhou escreveu fica em
`replaced-<hora>/` do volume. O código de saída é 0 para uma atualização concluída, 1 para uma revertida e 2
quando até a reversão falhou: a mensagem diz qual backup restaurar à mão.

Leia o [changelog](/changelog) antes. Um workspace migrado por uma versão mais nova não abre em uma mais
antiga, e por isso o backup vem primeiro.

### Fazer backup

```console
bash deploy/backup.sh
```

Grava `deploy/backups/stuarty-backup-<hora>.tar` e o `.sha256` dele. O servidor continua rodando: o banco é
copiado pelo backup online do SQLite, nunca como um arquivo bruto de um banco em escrita. O backup tem o
banco, seus arquivos originais, a biblioteca (memória, prompts e skills), as fontes, a tabela de preços dos
provedores e o cofre. Ficam de fora o que é refeito ou não é dado: exportações, proxies e quadros, logs,
ferramentas auxiliares e pastas de trabalho.

O comando por baixo é `stuarty workspace:backup <pasta>`. `manifest.json` lista cada arquivo com tamanho e
SHA-256 e diz `"master_key_included": false`. `SHA256SUMS` é a mesma lista para `sha256sum -c`.

### Restaurar em um servidor novo

1. Ponha sua chave mestra em `deploy/secrets/master_key`. O backup não a contém.
2. Rode `bash deploy/install.sh --domain <domínio> --email <e-mail> --no-start`. Ele grava a configuração e
   mantém a sua chave.
3. Copie o arquivo de backup (e o `.sha256`) para o servidor e rode `bash deploy/restore.sh stuarty-backup-<hora>.tar`.
4. Rode `bash deploy/install.sh` para iniciar.

```console
mkdir -p deploy/secrets && cp /lugar/seguro/master_key deploy/secrets/master_key
bash deploy/install.sh --domain stuarty.example.com --email voce@example.com --no-start
bash deploy/restore.sh stuarty-backup-20261119T120000Z.tar
bash deploy/install.sh
```

A restauração confere o arquivo pelo checksum, cada arquivo pelo manifesto e a chave mestra contra o cofre, e só
então grava. Uma chave que não abre o cofre interrompe com uma mensagem clara e não grava nada: ponha a chave
certa, ou use `--without-vault` para restaurar o resto e conectar as contas de novo. Um workspace que já tem
dados é recusado; `--replace` move esses dados para `replaced-<hora>/` antes e não apaga nada. Os contêineres
precisam estar parados.

### Guardar a chave mestra

A chave mestra é o único arquivo que não se recupera. Sem ela um cofre restaurado não abre: seus projetos, mídias
e biblioteca ficam intactos e você conecta as contas de novo. Copie `deploy/secrets/master_key` para um gerenciador
de senhas ou outro lugar seguro, longe do servidor e dos backups.

## Erros comuns

- Guardar os backups junto da chave. O backup tem o cofre selado; a chave o abre. Mantenha separados.
- Um domínio cujo DNS ainda não aponta para o servidor. O Caddy não consegue o certificado e o `install.sh`
  termina com uma mensagem que diz isso. Corrija o registro e rode de novo.
- Portas 80 e 443 fechadas no firewall do provedor.
- Atualizar de um checkout com alterações locais. O script recusa; faça commit ou descarte.
- Apagar a imagem da versão em execução. A reversão precisa dela.
- Dois servidores no mesmo volume. O segundo falha com `E_WORKSPACE_LOCKED`.
