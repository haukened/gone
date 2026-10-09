# Message catalogs

One file per language: `en.json` is the source, and `es.json`, `fr.json`, `de.json` and `pt-BR.json` translate it. The server renders pages from these files and serves them to the browser at `/i18n/<tag>.json`, so the language can change without a reload. The translations other than English were made by machine; corrections from native speakers are welcome (see [CONTRIBUTING.md](../../CONTRIBUTING.md#translations)).

## Message syntax

The files use the inlang message format.

- **Placeholders:** `{name}` is a value filled in at run time, such as a count, a size, a date or a file name. Keep every placeholder from English, spelled the same.
- **Slots:** `{#link}…{/link}` (also `strong`, `code`, `accent`, `opt`, `t`, `time`, `long`) wraps a link, emphasis or other markup that the page keeps. Translate the words inside and move the pair wherever it reads naturally. Don't add or remove pairs. An empty pair such as `{#time}{/time}` is filled in by the page.
- **Plurals:** a message that depends on a number is a one-item array:

  ```json
  [{"declarations": ["input count", "local countPlural = count: plural"], "selectors": ["countPlural"], "match": {"countPlural=one": "…", "countPlural=*": "…"}}]
  ```

  `*` is the "other" case. Add `countPlural=many` for languages that use it with large round numbers (Spanish, French and Portuguese: "1 000 000 de …").

## Conventions

- Keys say where text appears: `send.*`, `secret.*` and so on are page text; `js.*` is set by scripts; `common.*` is shared. A key used in two places means the same thing in both.
- "Gone" is never translated.
- Use typographic apostrophes (’) and the ellipsis character (…).
- **French:** a no-break space before `:` and a narrow no-break space before `?`, `!` and `;`.
- Keep security wording exact. If a sentence says what the server can or cannot do, or what someone with a link can do, the translation must make the same promise: no stronger, no weaker.

## Glossary

| English | Español (tú) | Français (vous) | Deutsch (du) | Português (Brasil) (você) |
|---|---|---|---|---|
| secret | secreto | secret | Geheimnis | segredo |
| link | enlace | lien | Link | link |
| one-time link | enlace de un solo uso | lien à usage unique | Einmal-Link | link de uso único |
| passphrase | frase de contraseña | phrase secrète | Passphrase | frase secreta |
| request (a secret) | solicitud / pedir | demande / demander | Anfrage / erbitten | pedido / pedir |
| manage link | enlace de gestión | lien de gestion | Verwaltungslink | link de gerenciamento |
| sender | remitente | expéditeur | absendende Person | quem enviou |
| recipient | destinatario | destinataire | empfangende Person | quem recebe |
| encrypt / decrypt | cifrar / descifrar | chiffrer / déchiffrer | verschlüsseln / entschlüsseln | criptografar / descriptografar |
| expire | caducar | expirer | ablaufen | expirar |
| Sealed · Waiting · Opened once · Gone (status steps) | Sellado · En espera · Abierto una vez · Borrado | Scellé · En attente · Ouvert une fois · Supprimé | Versiegelt · Wartet · Einmal geöffnet · Gelöscht | Lacrado · Aguardando · Aberto uma vez · Apagado |
