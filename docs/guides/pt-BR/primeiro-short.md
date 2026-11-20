# Seu primeiro short

O primeiro fluxo completo do Stuarty: um vídeo vertical curto, com legendas e miniatura, da ideia até a publicação. A versão em inglês, com cada comando, está em `site/content/docs/en/guide/first-short-video.mdx`; os comandos dela são executados por `scripts/docs-commands-smoke.sh`, e o fluxo inteiro, com serviços falsos, por `scripts/first-flow.sh`.

Cada passo tem o caminho no editor e o comando equivalente. Um agente usa os mesmos comandos pela CLI ou pelo MCP.

1. **Criar o projeto.** Em **Projetos**, **Novo projeto**, formato 9:16. Comando: `stuarty project:create --name "Short de pão" --id prj_5k2m9x4q1b`.
2. **Responder o briefing:** para quem é, o objetivo, a plataforma, o tom (`stuarty briefing:answer --project <id> --answer audience="..." --answer goal="..."`).
3. **Conectar um provedor de IA e rascunhar o roteiro.** Em **Conexões**, **Conectar**; a chave vai para o cofre. Em **Escrever**, peça o rascunho: ele vira um pedido em **Aprovações**, com a estimativa de custo, e espera você aprovar. Sem provedor, escreva o roteiro você mesmo.
4. **Importar o vídeo e cortar** (`stuarty media:import`, depois `stuarty edit` com `track.add` e `clip.add`).
5. **Legendas a partir da fala** (`stuarty transcribe`, depois a operação `caption.from_transcript`) e `stuarty lint`, que deve terminar sem erros.
6. **Miniatura a partir de um modelo**, em **Editar imagem** (`stuarty image:create --from-template`, `stuarty image:export`).
7. **Exportar com o preset da plataforma** (`stuarty export --preset youtube-shorts --wait`). A exportação é verificada contra os limites do preset.
8. **Conectar a conta** com o seu app de desenvolvedor da rede (`stuarty social:connect`).
9. **Criar a publicação e agendar** (`stuarty publish:validate`, `stuarty publication:create`, `stuarty publication:schedule`). A verificação contra os limites da rede roda antes de qualquer pedido.
10. **Aprovar.** Nada é enviado antes de você escolher **Aprovar** em **Aprovações**, e uma publicação cujo horário passa enquanto espera não sai sozinha. Depois da aprovação a publicação sai no horário e aparece no calendário de **Publicar** como publicada.

O TikTok recebe um rascunho, não uma publicação: você termina e publica no aplicativo do TikTok.
