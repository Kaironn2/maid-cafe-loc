# Regras para os agents de tradução (PT-BR)

Projeto: tradução não oficial do jogo "Maid Cafe on Electric Street" (café de maids em Akihabara, tom leve e fofo) para português do Brasil.

## Encoding e caracteres (OBRIGATÓRIO)
- Os arquivos são UTF-8 SEM BOM. Escreva acentos como caracteres reais (á é í ó ú â ê ô ã õ ç à), nunca como escape `\u00e7` nem como mojibake (Ã§, Ã£, Ã©, â€).
- Edite os arquivos com uma ferramenta de edição que preserve UTF-8. NÃO use PowerShell Set-Content/Out-File nem redirecionamentos `>` do PowerShell (eles corrompem o encoding).
- Depois de editar, rode SEMPRE o validador e corrija até sair "OK: nenhum problema.":
  node tools/check/check.js <arquivo que você editou>
- Abra o arquivo de novo e confira visualmente algumas linhas com acento.

## Formato
- Cada entrada é {"PT": "...", "EN": "..."}. Preencha SÓ o "PT". Nunca altere "EN", as chaves ou a ordem.
- Mantenha exatamente os marcadores {0} {1}, as tags <color=...>...</color> e uma quantidade parecida de \n do inglês.
- Aspas dentro do texto: \" (JSON válido).
- PT já preenchido é tradução revisada: é REFERÊNCIA OBRIGATÓRIA. Não altere essas entradas sem pedido explícito e siga os termos delas.

## Consistência (OBRIGATÓRIO)
- Antes de traduzir qualquer termo (menu, item, sistema, nome), procure se ele já foi traduzido:
  1. translations/glossary.md (glossário oficial);
  2. o PT já existente nos outros arquivos de translations/ (ex.: translations/texts/ui_en.json);
  Use exatamente a mesma tradução. Ex.: buscar com grep pelo termo em inglês e ver o PT correspondente.
- Nomes de personagens ficam como no inglês (Shiro, Miyu, Favna, Honoka, Rin...). Honoríficos e termos japoneses como Senpai, Kohai, Maid ficam em inglês/japonês como estão.
- Moeda: "yen" -> "ienes" (ou "¥" se o EN usar símbolo).
- Tom: português brasileiro natural, informal e simpático; menus e botões curtos (cabem pouco texto), em Title Case leve como já usado ("Novo Jogo", "Carregar Jogo").
