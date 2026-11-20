# Receita: um vídeo longo com uma pessoa falando

Este guia está em português por pedido do plano (épico 39); o texto de referência, em inglês, é a página "Recipe: a long talking video" da documentação (`site/content/docs/en/guide/recipe-long-talking-video.mdx`) e a especificação é a seção 17 do SDD-018.

Edite uma palestra, uma entrevista ou uma aula: uma abertura feita de um template, as pausas cortadas, legendas, capítulos, uma capa e um corte curto em 9:16 tirado do vídeo longo.

## O que você precisa

Um arquivo de vídeo de uma pessoa falando, `palestra.mp4`. A transcrição roda no seu computador com um modelo que você instala uma vez. Se você trabalha com um agente, peça que ele edite a palestra: as skills embutidas `long-talking-video` e `chapter-titles` conduzem os mesmos passos, e ele faz as perguntas do briefing.

## O briefing

Um vídeo longo tem perguntas próprias: o tema, o público, o objetivo, quem fala, a plataforma, o formato, a duração do vídeo editado (de 120 a 14.400 segundos) e o estilo de corte (`tight`, `natural` ou `relaxed`). Opcionais: os capítulos esperados, o que deve ficar mesmo que seja repetição ou pausa, se um corte curto deve ser extraído, o tom, referências e um prazo. Escreva-as em um arquivo Markdown que começa com `# Brief`, com um título `## <pergunta>` para cada resposta, e guarde-o:

```bash
stuarty project:create --name "Um ano de vídeos" --preset 16:9
stuarty brief:set --project prj_7k2m9x4q1b --text brief.md
```

## A abertura

Importe a palestra e transcreva. A transcrição é o que as legendas e os capítulos leem.

```bash
stuarty media:import palestra.mp4 --project prj_7k2m9x4q1b --wait
stuarty transcribe --project prj_7k2m9x4q1b --asset ast_4h6j8k2m3n --language pt --wait
stuarty media:inspect ast_4h6j8k2m3n --project prj_7k2m9x4q1b --sections transcript,silences
```

O template embutido **Long video opening** tem uma abertura fria (a melhor frase da palestra, uns seis segundos), um cartão de título e um terço inferior com o nome de quem fala. Seus espaços são `cold_open`, `title`, `subtitle` (opcional), `speaker` e `music` (opcional).

```bash
stuarty template:new tpl_r3c1p30003 --name "Um ano de vídeos, edição" --fill fill.json
```

## O corte principal

Ponha a palestra depois da abertura, na faixa `Footage` (`project:show --detail full` lista as faixas) e tire os silêncios. Os pedaços se fecham, então tudo o que vem depois anda para mais cedo.

```bash
stuarty edit --project prj_5d8f2k9m3q clip.add track_id=trk_0000000001 asset_id=ast_4h6j8k2m3n start=10s source_in=0s duration=290s
stuarty edit --project prj_5d8f2k9m3q clip.remove_silences item_id=itm_9k2m4p6r8t
```

O estilo de corte define quanto sai: acrescente `min_silence=0.3s` para um corte justo ou `min_silence=0.8s` para um corte relaxado. Uma repetição não é silêncio. Ache-a na transcrição, corte com `item.split` e remova o pedaço com `item.delete` e `ripple=track`. Nunca corte o que o briefing manda manter.

## Legendas e capítulos

Faça as legendas depois do corte, para que as palavras removidas não sejam legendadas:

```bash
stuarty edit --project prj_5d8f2k9m3q track.add kind=visual name=Captions
stuarty edit --project prj_5d8f2k9m3q caption.from_transcript clip_item_id=itm_9k2m4p6r8t track_id=trk_3c5v8n2w6d
```

Um capítulo começa onde o assunto muda, dura pelo menos 30 segundos, e o primeiro fica no início. Coloque um marcador em cada um e guarde a lista para a descrição como um texto do projeto, com a primeira linha em `00:00`:

```bash
stuarty edit --project prj_5d8f2k9m3q marker.add at=70s label="Por que um ano e não um mês"
stuarty item:add --project prj_5d8f2k9m3q --kind text --title "Capítulos" --text capitulos.txt
```

Os marcadores aparecem na linha do tempo. O arquivo exportado ainda não os leva como capítulos, então cole a lista na descrição do vídeo.

## Conferir, capa e exportação

```bash
stuarty lint --project prj_5d8f2k9m3q
stuarty export --project prj_5d8f2k9m3q --wait
```

A capa parte do template de imagem **Long video thumbnail** (`stuarty image:list --templates only`): um espaço de imagem, um título e um subtítulo. A skill `thumbnail` traz os passos.

## Um corte curto a partir do vídeo longo

Copie o projeto, para que o vídeo longo fique como está. Na cópia, corte no fim e no começo do trecho que você quer (30 a 60 segundos que se sustentam sozinhos) com `item.split`, apague o que fica fora com `item.delete` (`ripple=all` para o que vem antes, para que o trecho vá ao início), reenquadre e exporte:

```bash
stuarty project:duplicate prj_5d8f2k9m3q --name "Um ano de vídeos, curto"
stuarty edit --project prj_2n4p6r8t1v settings.reframe aspect=9:16
stuarty export --project prj_2n4p6r8t1v --wait
```

Um rosto tirado de um quadro largo pode se perder: olhe com `view` e ajuste com `clip.set_framing`. Faça as legendas de novo para o corte curto, maiores e em uma linha.

A receita inteira é executada de ponta a ponta, com um arquivo de cinco minutos e sem rede, por `bash scripts/recipe-long-video.sh`.
