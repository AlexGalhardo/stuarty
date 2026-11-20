# Stuarty: começar

O Stuarty é um estúdio de código aberto (MIT), que você roda no seu computador ou no seu servidor, para criar, editar, gerenciar e publicar seu conteúdo em um lugar só: vídeo, imagem e texto. Um agente de IA pode fazer tudo o que uma pessoa faz nele: tudo o que existe na interface existe também na linha de comando, na API HTTP e no MCP.

A documentação completa está no site, em inglês (ver `site/`, a decisão está na SDD-010 seção 15.1). Este guia reúne o que um brasileiro precisa para começar.

## O que fica na sua máquina

Seus arquivos, seus projetos, seus tokens e suas credenciais. Uma credencial fica no cofre do espaço de trabalho, e nenhuma página, comando ou agente consegue lê-la de volta. Sem nenhuma conexão configurada, o Stuarty não abre nenhuma conexão de saída e não tem telemetria.

## O que pode sair, e quando

Só o que você conecta de propósito:

- **Um provedor de IA** (chave de API, entrar com o ChatGPT, OpenRouter ou o seu agente local): recebe o texto e as imagens que você escolher enviar, com a sua chave.
- **Uma rede social** (YouTube, Instagram, Facebook, X, TikTok), pelo seu próprio app de desenvolvedor: recebe o vídeo, o texto e a miniatura **depois que você aprova** a publicação.
- **O login com o Google**, num servidor, se o dono ligar.
- Verificação de atualização do aplicativo de mesa, só se você disser que sim.

Publicar e gerar algo que custa dinheiro esperam por você na fila de aprovações. Um agente prepara, mas não aprova.

## Instalar

1. **Windows:** baixe `stuarty-desktop-X-Y-Z.exe` (aplicativo de mesa) ou `stuarty-X-Y-Z.exe` (servidor local, para linha de comando e agentes) na [página de releases](https://github.com/AlexGalhardo/stuarty/releases). O Windows avisa antes da primeira execução porque o instalador não tem certificado pago: escolha **Mais informações** e depois **Executar assim mesmo**. A assinatura e a soma de verificação provam que o arquivo é o genuíno.
2. **Docker:** `docker run -d --name stuarty -p 127.0.0.1:7878:7878 -v stuarty:/data -e APP_KEY="$(openssl rand -base64 32)" -e STUARTY_ADMIN_EMAIL=voce@exemplo.com -e STUARTY_ADMIN_PASSWORD="uma-senha-longa" ghcr.io/alexgalhardo/stuarty:X.Y.Z`. Publique a porta só em `127.0.0.1`.
3. **Servidor:** `git clone --branch vX.Y.Z https://github.com/AlexGalhardo/stuarty.git`, depois `bash deploy/install.sh --domain stuarty.exemplo.com --email voce@exemplo.com`. Guarde `deploy/secrets/master_key` fora do servidor: é o único arquivo que não se recupera.

O Stuarty usa o FFmpeg como processo separado. Se `stuarty doctor --full` disser que não achou, instale o FFmpeg 9 e coloque `ffmpeg` e `ffprobe` no `PATH`.

## Próximo passo

[Seu primeiro short](primeiro-short.md).
