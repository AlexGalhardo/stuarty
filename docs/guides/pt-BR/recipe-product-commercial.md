# Receita: um comercial de produto

Este guia está em português por pedido do plano (épico 39); o texto de referência, em inglês, é a página "Recipe: a product commercial" da documentação (`site/content/docs/en/guide/recipe-product-commercial.mdx`) e a especificação é a seção 17 do SDD-018.

Um comercial de 15 ou 30 segundos, feito a partir de um briefing, de um template embutido e de um arquivo exportado para cada proporção: 9:16, 1:1 e 16:9.

## O que você precisa

Três trechos curtos do produto (o gancho, o problema e o produto em uso) e, se tiver, uma música que seja sua. Um comercial de 30 segundos usa quatro trechos. Os identificadores abaixo são exemplos: use os que os comandos mostram.

Se você trabalha com um agente, não digite estes comandos: peça "um comercial de 15 segundos para o meu produto". As skills embutidas `product-commercial` e `commercial-copy` conduzem o agente pelos mesmos passos, e ele faz as perguntas do briefing uma de cada vez.

## O briefing

Um comercial tem perguntas próprias, diferentes das de um vídeo curto. As obrigatórias são o produto, o público, o problema que ele resolve, a oferta, a chamada para ação, a plataforma, o formato e a duração (de 5 a 90 segundos; 15 e 30 são as usuais). As opcionais são as tomadas que você já tem, a prova (avaliações, números, uma demonstração), o tom, referências, o que deve e o que não deve aparecer e um prazo.

Escreva as respostas em um arquivo Markdown cuja primeira linha é `# Brief`, com um título `## <pergunta>` para cada resposta, e guarde-o:

```bash
stuarty project:create --name "Caneca Keep" --preset 9:16
stuarty brief:set --project prj_7k2m9x4q1b --text brief.md
```

A skill `commercial-copy` transforma o briefing em cinco linhas: o gancho, o problema, o que o produto é, a oferta e a chamada para ação. Toda afirmação vem do briefing: um número ou uma avaliação que o briefing não traz nunca é escrito.

## A linha do tempo, a partir de um template

1. Importe as tomadas e anote os ids dos arquivos.

   ```bash
   stuarty media:import gancho.mp4 problema.mp4 produto.mp4 --project prj_7k2m9x4q1b --wait
   ```

2. Liste os templates. **Commercial 15 seconds** e **Commercial 30 seconds** são embutidos, com seus espaços.

   ```bash
   stuarty template:list
   ```

3. Preencha os espaços em um arquivo: cada nome de espaço leva `{"asset_id": "..."}` para uma tomada ou `{"text": "..."}` para uma linha. O template de 15 segundos tem as tomadas `hook_shot`, `problem_shot` e `product_shot`, as linhas `hook`, `problem`, `product_line`, `offer` e `call_to_action` e uma `music` opcional. O de 30 segundos tem duas tomadas do produto (`product_shot_1`, `product_shot_2`) e uma linha `proof`.

   ```bash
   stuarty template:new tpl_r3c1p30001 --name "Caneca Keep 9x16" --fill fill.json
   ```

   Use `--brand-color brand` para tomar a cor do seu kit de marca, quando o espaço de trabalho tem um. O briefing pertence ao primeiro projeto: guarde-o de novo no projeto novo com `brief:set`.

4. Confira. Um erro impede a exportação; o aviso `L_SAFE_AREA` importa, porque as plataformas desenham sobre as bordas de um quadro vertical.

   ```bash
   stuarty lint --project prj_5d8f2k9m3q
   stuarty view --project prj_5d8f2k9m3q --mode sheet --count 9
   ```

## Uma versão para cada proporção

Exporte o mestre, copie-o para que ele fique como está, reenquadre a cópia e exporte de novo:

```bash
stuarty export --project prj_5d8f2k9m3q --wait
stuarty project:duplicate prj_5d8f2k9m3q --name "Caneca Keep 1x1"
stuarty edit --project prj_2n4p6r8t1v settings.reframe aspect=1:1
stuarty export --project prj_2n4p6r8t1v --wait
```

Faça o mesmo com `aspect=16:9` em uma segunda cópia. O `settings.reframe` reajusta os itens ao novo quadro; uma tomada que preenche o quadro é cortada, então olhe cada cópia e ajuste o enquadramento com `clip.set_framing`. Leia as verificações de cada exportação: uma verificação que falha é uma exportação que falhou.

## Depois da exportação

O texto de cada rede sai da skill `caption-per-network`. Uma publicação vai para a fila de aprovação e espera por você; um agente não a aprova. Uma campanha paga é sua: o Stuarty prepara os arquivos e as palavras.

A receita inteira é executada de ponta a ponta, com material de teste e sem rede, por `bash scripts/recipe-commercial.sh`.
