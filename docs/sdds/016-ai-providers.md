# SDD-016: The AI provider layer (API key, text generation, cost)

- Status: Draft (2026-11-19). Written by epic 09, extended by epic 10 (image and voice, sections 2.2, 4.5, 5.1, 5.2, 6.1 to 6.3 and the lines marked epic 10); the code follows in the same epics.
- Depends on: ADR-0036 (one provider layer), ADR-0032 (outbound only to connected destinations; costly generation waits for a person), ADR-0033 (the vault), ADR-0018 (jobs), SDD-000 (names), SDD-007 (threat model, untrusted text), SDD-014 (connections and vault), SDD-015 (approval and cost)
- Implements: PRD-004 RF-360, RF-361, RF-362, RF-363, RF-364, RF-365; PRD-002 RF-133; NFR-063, NFR-064
- Owner area: `core/provider/`, `service/providers/` and `service/providers/<name>/`, the provider operations and the job kinds in `service/`, the controllers, commands and the job arguments in `web/app/`, the provider cards of the Connections page

## Contents

1. Overview
2. Providers and connections
3. The model catalogue
4. Prices and the cost model (`core/provider`)
5. The text request
6. The job `text.generate`
7. Service operations and who may call them
8. Errors
9. Safety
10. Surfaces
11. Tests
12. Sign-in connections (epic 12)

## 1. Overview

A creator connects the AI services they already pay for with an API key, once, and every part of Stuarty can use them. This document specifies the provider layer, its first capability, text, and (epic 10) image and voice generation. Video (epic 11), the sign-ins (epic 12) and the local agent (epic 14) add adapters and credential kinds to the same layer; none of them changes what is written here.

Four rules carry the design:

1. **A provider is reached only through a connection.** The kind of a connection (SDD-014 section 2) is the id of a provider. Every request goes through `connections.Manager.With`, so it passes the gate of `internal/outbound`: only the hosts of that connection, over https (or http on a loopback host, for the tests), and the key is read from the vault inside the closure and never leaves it. No adapter opens a socket.
2. **The price is shown before and recorded after.** A generation is a job of a kind that needs approval (SDD-015 section 5). Its estimate is computed in `core/provider` before the job exists, the ceilings of epic 05 can refuse it, and the real cost is recorded in the ledger when it ends.
3. **Nothing about a model is fixed in code.** The list of models is read from the provider at run time. A price is read from the catalogue of the provider when it publishes one, or from a table the person edits; a model with no price is refused, never estimated at zero.
4. **A provider answers with untrusted text.** The text a model wrote is untrusted (SDD-007 section 8); the error body of a provider is never shown.

What this epic does not do: no chat and no agent loop (one request, one answer, ADR-0037); no streaming (an adapter may offer it later behind the same interface); no image, voice or video; no sign-in (section 12, added by epic 12). Transcription (`engine/transcribe`, SDD-011 section 10) keeps working as it is, with its own provider settings; moving it onto this layer is left for the epic that adds the transcription capability.

## 2. Providers and connections

```go
// service/providers

type Descriptor struct {
	ID              string   // the connection kind: lower-case letters, digits and dash, 2 to 32 characters
	Name            string   // shown to a person
	Capabilities    []string // "text", "image" and "voice" (epic 10); "video" later
	CredentialKinds []string // "api_key" here
	DefaultEndpoint string   // the base URL a new connection starts from
	DocsURL         string   // where the person finds the key; shown, never called
}

type Provider interface {
	Descriptor() Descriptor
	// Models reads the catalogue of the provider with the credential of one connection.
	Models(ctx context.Context, c connections.Credential) ([]Model, error)
}

type TextGenerator interface {
	Provider
	// Estimate is the number of input tokens the provider will count for the request, as an
	// upper bound that needs no network (it adds the framing of the provider to core/provider's count).
	Estimate(req provider.TextRequest) provider.TokenEstimate
	// GenerateText makes one request and returns the text and the usage. It does not stream.
	GenerateText(ctx context.Context, c connections.Credential, req provider.TextRequest) (TextResult, error)
}

type ImageGenerator interface { // epic 10
	Provider
	// ImageLimits are the sizes and the count the provider documents for a model; needs no network.
	ImageLimits(model string) provider.ImageLimits
	// GenerateImages makes one request and returns the files, fetched through the gated client when the
	// provider answers with a URL. No file is larger than maxBytes.
	GenerateImages(ctx context.Context, c connections.Credential, req provider.ImageRequest, maxBytes int64) (ImageResult, error)
}

type VoiceGenerator interface { // epic 10
	Provider
	VoiceLimits(model string) provider.VoiceLimits
	// Voices lists the voices of the provider; a provider with no endpoint for them says what its documentation lists.
	Voices(ctx context.Context, c connections.Credential) ([]Voice, error)
	GenerateVoice(ctx context.Context, c connections.Credential, req provider.VoiceRequest, maxBytes int64) (VoiceResult, error)
}

// VideoGenerator is declared with no method and no adapter: epic 11 adds the methods and the adapters.
```

- **The registry** (`providers.Registry`) is built at start-up from the adapters: each adapter is registered by one line in the owner (`web/app/owner`). `Register` also registers the provider as the connector of its kind in `connections.Manager`, so a connection of kind `openai` can be created, listed and tested through the five operations of SDD-014. The connector's `Test` reads the model list (a harmless request) and reports the number of models in text the product wrote, or the kind of failure in a fixed sentence (section 8); it never forwards a body.
- **A provider is usable** when a connection of its kind exists and holds a credential. `ProviderList` says, for each provider, which connections it has.
- **A connection of a provider** has `kind` equal to the provider id, `endpoint` equal to the base URL (the descriptor's default, or another one for a server that speaks the same protocol), `hosts` defaulting to the host of the endpoint and `accepts` containing `text`. The credential is the API key (`api_key`, the only credential kind of this epic).
- **Adding a provider** is a new package `service/providers/<name>/` and one registration line. Nothing else lists the providers (`TestAddingAProviderNeedsOnePackageAndOneLine`).
- **Never** (RF-365, ADR-0036 item 3): an adapter that needs a subscription login a provider forbids, or a service with no official API. There is no Midjourney adapter and no adapter for a Claude.ai login; Anthropic is reached with an API key only.

### 2.1 The first adapters

Chosen in task 2 of the epic, proposed to the author for approval (the question is in the questions file of the author):

| Id | Provider | API | Models | Price in the catalogue |
| --- | --- | --- | --- | --- |
| `openai` | OpenAI | Chat Completions: `POST {endpoint}/chat/completions` with `Authorization: Bearer <key>`; request `model`, `messages`, `max_completion_tokens`, optional `temperature`; answer `choices[0].message.content`, `choices[0].finish_reason`, `usage.prompt_tokens`, `usage.completion_tokens` | `GET {endpoint}/models` | no |
| `anthropic` | Anthropic | Messages: `POST {endpoint}/messages` with `x-api-key: <key>` and `anthropic-version: 2023-06-01`; request `model`, `max_tokens`, optional `system`, `messages`, optional `temperature`; answer `content[]` blocks of type `text`, `stop_reason`, `usage.input_tokens`, `usage.output_tokens` | `GET {endpoint}/models?limit=1000` | no |

Neither catalogue publishes prices, so both use the table of section 4.2. The adapters use the standard library HTTP types and no vendor SDK. A provider that publishes prices (the catalogue of OpenRouter does) fills `Model.Price` and needs no entry in the table.

### 2.2 The providers of image and voice (epic 10)

Chosen with D-093 (the pair of providers is OpenAI and Anthropic) and the ones the epic asked the author to approve in its task 1. The adapter of OpenAI gains the two capabilities, in its own package, with no new registration line; Anthropic offers no image and no voice API, so its adapter stays a text provider and no capability is declared for it that it does not have.

| Capability | Endpoint of OpenAI | Request | Answer |
| --- | --- | --- | --- |
| `image` | `POST {endpoint}/images/generations` | `model`, `prompt`, `n`, `size`; no `response_format` (a model that needs one rejects it, and the adapter reads either answer) | `data[]` with `b64_json` or `url`, and `revised_prompt` for a model that rewrites the prompt |
| `voice` | `POST {endpoint}/audio/speech` | `model`, `input` (the text, at most 4,096 characters), `voice`, `response_format` (`mp3`, `wav`, `flac`, `opus` or `aac`) | the bytes of the audio |

- **Pricing.** Neither endpoint publishes a price, so both use the table of section 4.2, with a unit of its own (section 4.5). An image is paid per image and speech per million characters; the product knows no price.
- **Terms of use of the outputs.** Under the API terms of OpenAI the person who sends the request owns the output, as between the person and OpenAI, subject to the law and to the usage policies. The usage policies ask that a voice an API reads out be disclosed as AI-generated to the people who hear it, and forbid generating a voice or an image in the likeness of a real person without their consent. The product records provider and model in the origin of every file so a person can check the terms later, never treats the rights note as a licence (SDD-011 section 14.1), and shows the disclosure line in the Generate dialog of a voice. A person who connects another server that speaks the same protocol (the connection names its own base address) is bound by the terms of that service, which the product cannot read.
- **Voices.** OpenAI has no endpoint that lists voices; `Voices` returns the ones its documentation lists (alloy, ash, ballad, coral, echo, fable, nova, onyx, sage, shimmer, verse, marin, cedar), marked `source: documented`. A voice id the provider does not know is refused by the provider with `bad_request`. The voice cloning of a recording of a person is not offered.
- **A language** is accepted on a voice request and validated, and is not sent: the speech endpoint reads the language from the text. A provider that takes one sends it.
- **Models by capability.** The catalogue of OpenAI names models and nothing else, so the adapter gives each model a `capability` hint from its documented naming (`dall-e-*` and `gpt-image-*` for image, `tts-*` and `*-tts` for voice, the `gpt-*` and `o*` families for text; anything else has no hint). The hint only orders what a picker shows: a request names the model it wants and the provider is the judge.
- **A reference image** (an optional asset the model starts from) is not offered: the endpoint takes none, and an edit of an image with a model is epic 30.

## 3. The model catalogue

```go
type Model struct {
	ID            string          // 1 to 200 characters of [A-Za-z0-9._:/@+-], first one a letter or a digit
	Name          string          // a display name; untrusted text, cleaned and cut to 80 characters
	ContextTokens int             // the largest input plus output, 0 when the provider does not say
	OutputTokens  int             // the largest output, 0 when the provider does not say
	Price         *provider.Price // set when the catalogue publishes a price
	Capability    string          // epic 10: "text", "image", "voice" or empty; a hint from the adapter, never a rule
}
```

- The catalogue is read from the provider with `Models` on every `ProviderModels` call that has no fresh copy. A copy is kept in memory per connection for 10 minutes (`CatalogueTTL`); `refresh: true` asks for a new read. A copy is never served for a connection that no longer exists: the connection is checked first. Nothing is written to disk.
- A model whose id does not match the pattern above is dropped, so an id can be used in a command line and in a prompt without carrying text. At most 2,000 models are kept, sorted by id.
- **The estimate of a job never calls the provider** (SDD-015 section 5: `Prepare` makes no outbound call). It uses the copy in memory when there is one (limits and a published price) and otherwise the table of section 4.2 alone.

## 4. Prices and the cost model (`core/provider`)

All of it is pure: no I/O, no clock, no network.

### 4.1 Money and tokens

A price is `provider.Price{Currency, InputMicrosPerMTok, OutputMicrosPerMTok}`: the cost, in millionths of the currency unit, of one million input tokens and of one million output tokens. `Currency` is three upper-case letters. Both amounts are non-negative and at most 10^12 (a million units per million tokens).

- `TextCost(usage, price)` is `ceil(input_tokens * InputMicrosPerMTok / 10^6) + ceil(output_tokens * OutputMicrosPerMTok / 10^6)`: each side is rounded up, so a cost is never rounded to nothing. It uses 128-bit-safe integer arithmetic (`math/bits`), so an absurd token count cannot overflow into a small number.
- `EstimateText(req, price, framing)` is the upper bound of a request: the input tokens are `ceil(bytes(system) + bytes(prompt)) / 3` plus `framing` (the adapter's constant for the message framing), and the output tokens are `req.MaxTokens`, the most the provider may produce. The estimate is a bound, not a promise: tokenizers differ, and the ledger records the real cost, which may be above the estimate by the error of the input count (SDD-015 section 6). Three bytes per token is deliberately pessimistic for English and about right for Chinese and Japanese.

### 4.2 Where a price comes from

In this order, per provider and model:

1. **The table the person edits.** `<workspace>/providers/prices.json` (section 4.3). An entry is the person's word: it wins over the catalogue.
2. **The catalogue of the provider**, from the copy in memory (section 3), when the provider publishes a price.
3. **None.** The request is refused with `E_PRICE_UNKNOWN` (section 8), whose hint says which model has no price and how to set one. No price is built into the code, and a missing price is never taken as zero, because a zero estimate would pass every ceiling.

`ProviderModels` shows, for each model, the price that would be used and where it comes from (`price_source`: `user`, `catalogue` or `none`).

### 4.3 The price table

One file, `<workspace>/providers/prices.json`, mode 0600, written through a temporary file and a rename:

```json
{
  "format": "stuarty-prices",
  "version": 1,
  "prices": [
    { "provider": "openai", "model": "a-model-id", "currency": "USD",
      "input_micros_per_mtok": 2500000, "output_micros_per_mtok": 10000000 }
  ]
}
```

At most 500 entries, one per provider and model: a file edited by hand with two entries for one model is refused as a whole, like any entry that does not pass the rules, since the estimate would otherwise use whichever came first. The file has no secret. A person sets or removes an entry with `ProviderPriceSet` (section 7): because a lower price lowers every estimate, it is an operation of a privileged browser session, like the ceilings of SDD-015 section 4, so an agent cannot talk its way under a ceiling by editing a price.

### 4.4 Ceilings and approval

The estimate goes to `core/approval` `Decide` as the `Cost` of the action (kind `text.generate`, the connection, the project), so the workspace and project ceilings of epic 05 apply as they do to every costly action: a request above a ceiling is refused with `E_COST_CEILING` and nothing is written; any other request waits for a person unless a rule the person wrote covers it (a rule needs a per-item limit to approve a costly action). This epic adds no rule of its own to the decision.

### 4.5 Prices of an image and of speech (epic 10)

A price has a `unit`. The default (empty) is the price of tokens of sections 4.1 and 4.3, and an old table has no other. Two units are added; an entry of either has `input_micros_per_mtok` and `output_micros_per_mtok` at zero and sets `unit_micros`:

| `unit` | `unit_micros` is | Used by |
| --- | --- | --- |
| `image` | the cost of one image | `image.generate` |
| `mchar` | the cost of a million characters of the text of a speech | `voice.generate` |

```json
{ "provider": "openai", "model": "an-image-model", "currency": "USD", "unit": "image", "unit_micros": 40000 }
```

- `ImageCost(count, price)` is `count * unit_micros`; `VoiceCost(characters, price)` is `ceil(characters * unit_micros / 10^6)`, in 128-bit arithmetic as `TextCost` is, so a speech of one character at a nonzero price is never rounded to nothing (`ImageCost` saturates at the largest int64). A price whose unit does not fit the capability of the job (a token price for an image) is the same as none: the request is refused with `E_PRICE_UNKNOWN`, whose hint says which unit is wanted.
- The rules of sections 4.2 to 4.4 apply unchanged: the table of the person wins over a catalogue, a missing price is never zero, only a privileged browser session sets a price, and the estimate goes to `Decide` as the `Cost` of the action.
- An estimate of an image is exact for the images the provider returns, and an upper bound when the provider returns fewer (a partial batch costs what arrived, section 6.2). The estimate of speech counts the characters of the text, so it is exact for a provider that bills by character; a provider that bills by the second of audio is priced by the person in the table at the rate the person finds fair, since the real cost is recorded from the price the person saw (section 6.1).

## 5. The text request

```go
type TextRequest struct {
	Model       string   // required
	System      string   // optional instructions
	Prompt      string   // required
	MaxTokens   int      // the most the provider may produce; default 1024
	Temperature *float64 // optional, 0 to 2
}
```

`ValidateText(req, limits)` checks, with `E_VALIDATION` and the path of the member:

| Member | Rule |
| --- | --- |
| `model` | the pattern of section 3 |
| `prompt` | 1 to 100,000 characters of valid UTF-8, no NUL |
| `system` | at most 20,000 characters of valid UTF-8, no NUL |
| `max_tokens` | 1 to 32,768, and not above `OutputTokens` of the model when the catalogue copy says |
| `temperature` | 0 to 2 |
| the whole | the estimated input plus `max_tokens` not above `ContextTokens` of the model when the catalogue copy says |

The prompt and the system text are text a person or an agent wrote, and may hold untrusted text (a transcript, a note): they are sent as they are to the provider the user chose and to no other destination, and they are never put in a log. What is shown to a person is in section 6.

### 5.1 The image request (epic 10)

```go
type ImageRequest struct {
	Model  string // required
	Prompt string // required
	Size   string // "<width>x<height>"; default "1024x1024"
	Count  int    // images wanted; default 1
}
```

`ValidateImage(req, limits)` checks, with `E_VALIDATION` and the path of the member: `model` (the pattern of section 3); `prompt` (1 to 4,000 characters of valid UTF-8, no NUL); `size` (two whole numbers from 64 to 4,096 joined by `x`, and one of `limits.Sizes` when the provider documents the sizes of the model); `count` (1 to 10, and not above `limits.MaxCount` when documented). `ImageLimits` (`Sizes`, `MaxCount`) is what the adapter documents for the model; zero means not documented. Limits are read with no network, so `Prepare` stays free of outbound calls.

### 5.2 The voice request (epic 10)

```go
type VoiceRequest struct {
	Model    string // required
	Text     string // required: what is read out
	Voice    string // required: a voice id of the provider
	Language string // optional: a BCP 47 tag such as pt-BR
	Format   string // mp3 (the default), wav, flac, opus or aac
}
```

`ValidateVoice(req, limits)` checks: `model` as above; `text` (1 to `limits.MaxChars` characters, 4,096 when the provider documents none, valid UTF-8, no NUL); `voice` (1 to 100 characters of letters, digits and `. _ : / @ + -`, the first a letter or a digit); `language` (empty, or a tag of letters, digits and dashes starting with two or three letters, at most 35 characters); `format` (one of the five, and one of `limits.Formats` when documented).

### 5.3 What a result must be (epic 10)

A file a provider returns is checked before it becomes an asset, by `provider.CheckImage` and `provider.CheckAudio` (pure, over the first bytes): an image is a PNG, a JPEG or a WebP by its signature, not empty and not above the import limit of the workspace; audio has the signature of the format that was asked for (an MP3 with an ID3 tag or a frame sync, `RIFF`/`WAVE`, `fLaC`, an Ogg page, an ADTS header), under the same limit. A file that is neither is `E_PROVIDER` with `details.kind` `unreadable`, and nothing is stored. The ingest of SDD-011 then probes the file as it does any other, so a file that passes the signature and is not decodable still ends as a failed asset with its reason, and no more than the bytes already counted were written.

## 6. The job `text.generate`

Epic 17 registers a second kind of the same shape, `text.draft` (SDD-019 section 6): it is `text.generate` with a prompt that `core/writing` builds from the brief, the current text and an instruction, and with a text to put the answer in as a new revision. It has the estimate, the summary, the ceilings, the approval and the ledger described here, and its result is the result of this section with `item_id` and `revision`.

`text.generate` is registered in `service.Options.JobKinds` by the service when a provider registry is configured (SDD-015 section 5). Its scope is `edit` (it writes text into a project). Its arguments (`JobStartRequest.Args`, and the body of `TextGenerate`):

| Argument | Rule |
| --- | --- |
| `connection_id` | required; a connection whose kind is a provider with the text capability and whose `accepts` has `text` |
| `model`, `prompt`, `system`, `max_tokens`, `temperature` | section 5 |

- **Prepare** validates the arguments, finds the price (section 4.2) and computes the estimate. It makes no outbound call. The **summary** a person reads is made by the product: the provider and the model, the size of the prompt in characters, the largest output, and an excerpt of the first 200 characters of the prompt with every control, invisible and separator character removed and white space collapsed (SDD-015 section 3 refuses a summary that has them). The summary is shown as untrusted text. The input stored in the job is the arguments, the estimate and the price used, so that what is spent is judged with the price the person saw.
- **Run** reads the credential with `connections.Manager.With` (a timeout of 120 seconds for a generation; the test request keeps the default), calls `GenerateText`, and returns the result and the real cost, `TextCost(usage, price)`. A provider that fails returns no cost, so the reservation of the ledger is released; a provider that answered and reported usage is settled at that cost.
- **The result** is stored on the job (`result.text`): `provider`, `model`, `text`, `finish_reason` (`stop`, `length` or `other`), `input_tokens`, `output_tokens` and `cost` (`micros`, `currency`). `text` is untrusted text; `model` is the model the answer names when it matches the pattern of section 3, else the model that was asked for. Items of a project that hold text are written by the epics that own them; until then the caller reads the text from the job, with `JobGet`.
- **The estimate before the job.** The job view carries `estimate` (`micros`, `currency`) from the moment the job exists, also for `dry_run` and for a job that waits for approval; its `text` says it. The approval request of epic 05 carries the same estimate.
- The ledger (SDD-015 section 6) records the estimate when the job is approved and the real cost when it ends.

### 6.1 The jobs `image.generate` and `voice.generate` (epic 10)

Both are approvable kinds of scope `edit`, registered like `text.generate` and passing through the same estimate, ceiling and approval (SDD-015 section 5). Arguments (`JobStartRequest.Args`, and the body of `ImageGenerate` and `VoiceGenerate`):

| Kind | Arguments |
| --- | --- |
| `image.generate` | `connection_id` (a connection to a provider with the `image` capability whose `accepts` has `text`, since only text is sent), and the members of section 5.1 |
| `voice.generate` | `connection_id` (capability `voice`), and the members of section 5.2 |

- **Prepare** validates the arguments with the limits of the adapter, finds the price of the model in the unit of the capability (section 4.5) and computes the estimate; it makes no outbound call. The summary is made by the product: the provider and the model, what is asked for (the count and size of the images, or the number of characters, the voice and the format of the speech) and an excerpt of the first 200 characters of the prompt or the text, cleaned as in section 6. The input stored in the job holds the arguments, the price used and the estimate.
- **The project.** The job belongs to a project (`project_id`, a production or a timeline of it, as `MediaImport` takes it): the library is the view of the files that projects hold (SDD-011 section 14), so a file with no project would be in no list. The epic that lists "an optional `project_id`" is refined to required for that reason.
- **Run** checks that the disk of the workspace has room for the largest result (the guard of SDD-011 section 2.3 step 3, before any request), reads the credential with `connections.Manager.With` (a timeout of 180 seconds for images and 120 seconds for speech), calls the adapter and checks every file (section 5.3). It then stores each file in the media store, content-addressed, and links it to the project as an asset of the same flow as an import (SDD-011 section 2.3), with the origin `generated`: `provider`, `model`, `prompt` (the prompt or the text, cut to the limit of the library, untrusted), `cost_micros` (the cost of that file; for an image the price of one, for speech the cost of the text) and the actor of the job as the creator (`created_by`). The ingest then runs as for any file, and the job waits for it for up to 60 seconds, so that it ends with assets a project can use (`items[].state` is `ready`); a file whose ingest fails is stored, paid for and said in a note.
- **A URL result.** An adapter that receives a URL downloads it with the gated client of the connection only: a host the connection does not list is refused by the gate, and the job reports `E_CONNECTION_DESTINATION`. A download is read to `maxBytes` and no further. Nothing else in the product fetches it.
- **The result** (`result.generated`): `provider`, `model`, `items` (each with `asset_id`, `hash`, `kind`, `bytes` and, for an image, `revised_prompt`, untrusted text like the prompt), `requested`, `delivered`, `notes` (text the product wrote about what did not arrive) and `cost` (`micros`, `currency`). The job `text` lists the items.
- **The ledger.** The estimate is recorded at approval and the real cost at the end: the price the person saw times what arrived (images delivered, characters of the text of a delivered speech). A request that fails records no cost and releases the reservation.

### 6.2 Failures and a partial batch (epic 10)

A refusal of the prompt by the provider (a content policy) is `E_PROVIDER` with `details.kind` `refused`; a rate limit is `rate_limited` (with `retry_after_seconds`); a request that did not finish in time is `timeout`; each is a distinct stable error. A batch of images that delivers some of the files keeps what arrived: the job succeeds with the items it has, `delivered` below `requested`, a note for the rest, and the cost of what arrived. A batch that delivers none fails with the error of the first failure.

### 6.3 Voices (epic 10)

`VoiceList` reads the voices of a connection (`id`, `name`, `language`, `source` of `provider` or `documented`) through the connection, and costs nothing. The voice names are untrusted text, cleaned and cut like the names of models (section 3), and a name that echoes the key is dropped (section 9).

## 7. Service operations and who may call them

Four operations on `service.Service`, in `service/provider_ops.go`, and three of epic 10 in `service/generate_ops.go`:

| Operation | Scope | Result |
| --- | --- | --- |
| `ProviderList` | read | `providers`: each descriptor with the ids of its connections; `prices`: the entries of the price table |
| `ProviderModels` | read | the models of one connection, read from the provider (`refresh` forces a new read): each with `id`, `name`, `context_tokens`, `output_tokens`, `price` and `price_source` |
| `ProviderPriceSet` | admin, and only from a privileged browser session | the price table after the entry was set; with both amounts empty the entry is removed |
| `TextGenerate` | edit | the job (`JobStart` of `text.generate`); `dry_run` answers the job it would create, or the error, and writes nothing |
| `ImageGenerate` | edit | the job (`JobStart` of `image.generate`); `dry_run` as `TextGenerate` |
| `VoiceGenerate` | edit | the job (`JobStart` of `voice.generate`) |
| `VoiceList` | read | the voices of one connection |

- `ProviderPriceSet` takes `unit` (empty, `image` or `mchar`) and `unit_micros` besides the members of section 4.3 (epic 10).
- `ProviderPriceSet` is checked like `CostCeilingSet`: a caller that is not a privileged browser session gets `E_PRIVILEGED_SESSION` whatever scope it holds; then the scope. It is audited (`provider.price.set`, with the provider, the model and the amounts).
- A project-limited key is refused `ProviderList`, `ProviderModels` and `ProviderPriceSet` (they name no project) and may call `TextGenerate` for its project only.
- No operation returns a key, and none takes one: the key is given once, to `ConnectionCreate` (SDD-014).

## 8. Errors

Added to SDD-000 section 8:

| Code | Meaning | HTTP |
| --- | --- | --- |
| `E_PROVIDER` | the provider refused or failed a request; `details.kind` is `auth` (the key was refused), `rate_limited`, `bad_request` (the provider refused the request; `details.code` carries the provider's short error code when it is a safe token), `unavailable` (5xx or no answer), `unreadable` (an answer that is not the documented shape, or a generated file that is not what it must be, section 5.3), `refused` (the provider declined the prompt under its content policy; epic 10) or `timeout` (the request did not finish in time; epic 10); `details.status` is the HTTP status when there was one, and `details.retry_after_seconds` is set when the provider gave one. The body of the provider is never in the error | 502 |
| `E_PRICE_UNKNOWN` | no price is known for the model: neither the table nor the catalogue has one; `details` hold `provider` and `model`; the hint names `provider:price` and the Connections page | 422 |

A result URL on a host outside the destinations of the connection is `E_CONNECTION_DESTINATION` (SDD-014 section 7). A missing or removed connection is `E_CONNECTION_NOT_FOUND`; a connection whose kind is not a provider of the capability of the job is `E_VALIDATION` on the path `connection_id`; a bad field is `E_VALIDATION` with `path`. An error text is written by the product from the kind and the status, never from the body of the provider, and holds no key.

## 9. Safety

- **The key.** Read inside `Manager.With`, used in one header, cleared when the closure returns, never in a log, an error, a result, a job input or an audit row. Adapters build their errors from the status and from a short code that matches `[a-z0-9_.-]{1,64}`, never from the body or from a Go error that could wrap a header.
- **The prompt as untrusted input.** A prompt may carry text from a transcript or a file name. It goes only to the destination of the connection; the approval summary shows an excerpt cleaned of invisible characters (T-52).
- **The output as untrusted text.** The text is delimited as network text wherever a text view shows it to an agent (SDD-007 section 8), is never run, never fed to a tool by the service and never written into a document by this epic (T-53).
- **The cost.** Every generation has an estimate before it exists; the price table is for a person only; a model with no price is refused; `max_tokens` bounds the output; the ceilings are checked when the job is submitted, when a person approves it and again before it spends; the number of requests waiting for a person is bounded per submitter (T-54).
- **The error body.** A provider may echo the prompt or the key in an error. The body is read to a bound of 8 KiB to take the short code and is then dropped (T-55).
- **An echo.** A server that speaks the protocol, such as one a connection names with its own base address, may repeat the key it was sent. The text of an answer, the id and the name of a model and an error code are checked for the key, or any eight consecutive bytes of it, in the forms of SDD-014 section 9 (`Credential.Echoes`): a model that echoes it is not listed, an answer that echoes it is replaced by a fixed sentence (the cost is still recorded), and a code that echoes it is not shown (T-55).

- **Generated media (epic 10).** A file is untrusted input: it is fetched only through the gated client (a result URL on a host the connection does not list is refused), read to a bound, checked by signature before it is stored, and then probed by the ingest like any import. The prompt and the revised prompt are stored as untrusted text in the origin. The key never appears in a revised prompt, a voice name or a note: each is checked for it as the text of an answer is.

## 10. Surfaces

- HTTP: `GET /providers`, `GET /connections/{id}/models` (query `refresh`), `PUT /providers/prices`, `POST /text/generate` (status 202, as `POST /projects/{id}/jobs`: the job is the answer).
- CLI: `provider:list`, `provider:models <connection id>`, `provider:price` and `text:generate`. The API key is added with `connection:add --kind <provider id>` (SDD-014), which reads it from standard input or a file. `text:generate` takes the prompt from `--prompt`, `--prompt-file` or standard input, and answers the job with its estimate, without waiting for it: a generation waits for a person, so there is no `--wait`. `provider:price` needs a privileged session, so with the local token it always fails with `E_PRIVILEGED_SESSION`, which gives an agent the stable error and its hint. The four commands are in the `provider` domain (SDD-000 section 1.1): the guide of that domain (`agent:docs --print guide:provider`) lists the three an agent may run, and `stuarty provider <verb>` runs them. They are not in the generated skill for agents, whose 12,400 characters have no room left.
- MCP: no new tool. A new domain adds no tool (SDD-003 section 3.11, as revised by epic 13). An agent starts a generation with `job_start` (kind `text.generate`, arguments `connection_id`, `model`, `prompt` and optionally `system`, `max_tokens`, `temperature`) and reads it with `job_get`. The description of `job_start` names the three required arguments, and a start with no `connection_id` is refused with a hint that names every argument. To fit the budget, the `kind` enum of `job_start` gains `text.generate` and the `state` of the `outputSchema` of `job_start` and `job_get` becomes a plain string (it lists `waiting_approval`, which the five-value enum did not; SDD-003 section 4.2, SDD-015 section 10). `provider:list` and `provider:models` have no tool: a provider is a connection (`connection`), and the models are read with the CLI or HTTP.
- Epic 10 adds, on HTTP: `POST /image/generate` and `POST /voice/generate` (status 202, the job is the answer) and `GET /connections/{id}/voices`; on the CLI: `image:generate`, `voice:generate` (the text from `--text`, `--text-file` or standard input) and `voice:list <connection id>`, in the `provider` domain, with the same rules as `text:generate` (no `--wait`); on MCP: no new tool, the `kind` enum of `job_start` gains `image.generate` and `voice.generate`, and `voice:list` has no tool, like the models. The `format` member of `args` is shared with the export formats and its `enum` is exactly those (`TestJobStartFormatsMatchTheService`), so over MCP a narration is asked in `wav` or `aac`, or in the default `mp3` by leaving `format` out; `flac` and `opus` are asked over HTTP or the CLI.
- The Generate dialog (epic 10) is in the two sections of the asset library (`VisualAssets` for an image, `VideoAssets` for speech). It asks for the connection, the model (the models whose capability hint matches come first), the prompt or the text, the options, and shows the estimate from a `dry_run` before anything is submitted; then the job (it may wait for approval) and, when done, the files in the grid. For a voice it shows the disclosure line of section 2.2. The dialog computes nothing.
- The Connections page shows one card per provider: connect with a key (the key goes to `ConnectionCreate`), test it, the models with their price and its source, a form to set a price for a model, and the spend of the day of each connection from `CostSummary`. The page computes nothing: amounts are formatted, not summed (SDD-012 section 1).

## 11. Tests

| What | Test |
| --- | --- |
| Validation, estimate and cost tables, including overflow and rounding | `core/provider` table tests |
| Each adapter against recorded request and response pairs, including the key header, errors and rate limits | `service/providers/<name>` contract tests over a fake server |
| The catalogue is read at run time and cached, and a removed connection is not served | `service/providers` tests |
| Adding a provider needs one package and one line | `TestAddingAProviderNeedsOnePackageAndOneLine` |
| A job completes and stores the text; the estimate is shown first; the cost is recorded; a ceiling refuses; a rule approves | `TestTextGenerate_*` over the Service |
| A prompt with hostile text is shown cleaned in the summary | `TestSecurity_T52_PromptIsNeverShownRawToAPerson` |
| A hostile answer is untrusted text and nothing runs it | `TestSecurity_T53_ProviderOutputIsUntrustedText` |
| Cost run-away: unknown price, an agent cannot edit a price, the ceiling | `TestSecurity_T54_CostCannotRunAway` |
| A key or a provider error body never reaches a log, an error or a result | `TestSecurity_T55_KeyAndErrorBodyNeverLeave` |
| The surfaces agree | `web/contract` parity |
| Connect with a fake provider server, generate, see the cost | `web/e2e/specs/providers.spec.ts` |
| Image and voice: limits, estimates, signatures of results | `core/provider` table tests (epic 10) |
| The OpenAI adapter against recorded requests and answers: a base64 image, an image by URL, a refused prompt, a rate limit, the speech bytes, the voices | `service/providers/openai` contract tests (epic 10) |
| An image and a voice job: estimate first, a ceiling that waits for approval with no outbound call, assets with a complete origin, a partial batch, a URL on a foreign host | `TestImageGenerate_*`, `TestVoiceGenerate_*` over the Service (epic 10) |
| A generated narration is placed on a timeline and exported | `TestGeneratedVoiceOnTimeline` |
| The surfaces agree on the three operations | `web/contract` parity (epic 10) |
| The Generate dialog against a fake provider server | `web/e2e/specs/generate-assets.spec.ts` |

No test calls a real provider. The recorded responses are written by hand from the public documentation of each API and kept in `testdata/` next to the adapter; the manual check with a real key of the author (task 8) is not part of the suite.

## 12. Sign-in connections (epic 12)

Two providers can be connected with a sign-in in the browser instead of an API key: **Sign in with ChatGPT** (provider `openai`, credential kind `oauth`) and **OpenRouter** (provider `openrouter`, credential kinds `api_key` and `oauth`). The credential of a sign-in lives only in the vault, like a key (SDD-014 section 12). A person who has no such account, or who prefers a key, keeps the API key path of section 2. There is **no** sign-in with a Claude.ai subscription, or with any login a provider forbids third-party applications to use (RF-365).

### 12.1 What the providers document (read on 2026-11-19)

| Page | Used for |
| --- | --- |
| <https://developers.openai.com/siwc/quickstart> | the open-source clients flow, the scopes, no client secret |
| <https://developers.openai.com/siwc/token-sharing-open-source/sign-in> | endpoints, parameters, registration, refresh |
| <https://developers.openai.com/siwc/token-sharing-open-source/models-and-inference> | model list and inference with the plan token |
| <https://developers.openai.com/siwc/token-sharing-open-source/self-hosted-vms> | host identifier, credential transfer, the loopback limit |
| <https://openrouter.ai/docs/guides/overview/auth/oauth> | the PKCE flow, the key, the callback rules |

**OpenAI (Sign in with ChatGPT, the flow for open-source and locally hosted clients).** It is an OpenID Connect authorization code flow with PKCE (`S256`), for a public client: no client secret and no partner key.

- Authorization: `https://auth.openai.com/api/accounts/authorize`, opened in the system browser. Token: `https://auth.openai.com/api/accounts/oauth/token`, a form-encoded `authorization_code` grant.
- **Registration.** The first sign-in on a host uses `client_id=dynamic_agent_client` with `ext_agent_host_id` (a stable identifier of the host, distinct for each host, kept and reused) and `agent_name_hint` (the name of the application). After the person names and authorises the agent, the callback carries the **issued** client id (`oaiapp_...`) in the `client_id` query parameter. It is kept with the sign-in and used for every later request of that account; `dynamic_agent_client` is never kept, a callback with no issued id is an incomplete registration, and a different id on a later sign-in is refused.
- **Parameters** of the authorization request: `client_id`, `agent_name_hint`, `ext_agent_host_id`, `response_type=code`, `redirect_uri`, `scope`, `resource=https://api.openai.com/v1`, `state`, `nonce`, `code_challenge_method=S256`, `code_challenge`. Scopes: `openid profile email offline_access resource.invoke chatgpt.tokens.use.direct`. The plan can be used only when `chatgpt.tokens.use.direct` is granted. The code exchange repeats `redirect_uri` and `resource`.
- **Redirect.** An HTTP loopback address on `127.0.0.1`, `http://127.0.0.1:<port>/auth/callback`; only the port may change between sign-ins, `localhost` is not accepted, and the same address is sent in both requests. HTTPS and custom schemes are not documented.
- **Tokens.** The answer carries the access token, a rotating refresh token (requested with `offline_access`), `expires_in` and an ID token. After a refresh the access token, the expiry, the scopes and the refresh token are replaced together. The ID token is checked for issuer, audience (the issued client id), expiry and nonce.
- **Use.** `GET https://api.openai.com/v1/models` with `Authorization: Bearer <access token>` answers `{"models": [...]}`; an entry is shown when `visibility` is `list`, its `slug` is the model id and `display_name` the name. `POST https://api.openai.com/v1/responses` (the public Responses API, never the `backend-api` of ChatGPT) with `store: false` and `stream: true`; the request counts as successful only after the event `response.completed`; `response.failed` may carry `subscription_sharing_usage_limit_exceeded` or `subscription_sharing_usage_unavailable`. The model list is read again when the account changes.
- **Terms.** The plan is available to "all open-source partners and selected private clients"; Stuarty is open source (MIT) and a local application, so the use is inside what the pages document. The terms and eligibility page was not reachable on the date (a 404), and the pages do not say whether a server shared by several people is covered: see section 12.3.
- **Not documented on the pages read:** the refresh request (the standard `refresh_token` grant on the token endpoint is used, with the issued client id), the JWKS address, rate limits and the models a plan grants.

**OpenRouter (OAuth PKCE).**

- Authorization address: `https://openrouter.ai/auth?callback_url=<address>&code_challenge=<challenge>&code_challenge_method=S256&state=<state>`. `code_challenge_method` must be `S256`. The `state` is echoed on the callback, and is not returned when the person denies.
- Callback: `https://` for a hosted application, or `http://localhost:<port>/...` on any port. The page does not say whether `127.0.0.1` is accepted; the address of the page the person is at is used as it is.
- Exchange: `POST https://openrouter.ai/api/v1/auth/keys` with a JSON body `{"code", "code_verifier", "code_challenge_method": "S256"}`; the answer has `key`, a key of the person bound to their account, which spends their credits until they revoke it. The code lives 10 minutes and is single-use. Errors: 400 (method), 403 (invalid or expired code or verifier), 405.
- Use: `Authorization: Bearer <key>` on `https://openrouter.ai/api/v1/chat/completions`; the catalogue is `GET /api/v1/models`, whose `pricing.prompt` and `pricing.completion` are USD per token, as decimal text. No terms of third-party applications are on the page.

Neither provider changed in a way that forbids this use: task 1 of the epic does not stop.

### 12.2 The adapters and the credential

- **`openrouter`** (`service/providers/openrouter`). The Chat Completions wire of OpenRouter: `POST {endpoint}/chat/completions` with `model`, `messages`, `max_tokens`, optional `temperature`; the answer has the shape of section 2.1 (the model the answer names, the text, the finish reason and `usage`). `Models` reads `GET {endpoint}/models`: `id`, `name`, `context_length`, `top_provider.max_completion_tokens` and the price, which the catalogue publishes: a price of `-1` (a router with a variable price) or one that is not a non-negative decimal gives no price, so that model is refused as in section 4.2. The price in micros per million tokens is the decimal price per token times 10^12, rounded up. The key is the secret of the connection, whether the person typed it or the sign-in made it; the adapter does not tell them apart.
- **`openai` with a sign-in.** The same adapter, with the credential kind `oauth`: the connection manager says it (`Credential.SignedIn`, set from the secret in the vault) and hands the adapter the access token as the secret. The adapter uses it as the bearer and speaks the Responses API of section 12.1. `GenerateText` sends `model`, `input` (the prompt), `instructions` (the system text, when there is one), `max_output_tokens`, optional `temperature`, `store: false` and `stream: true`, reads the events, joins `response.output_text.delta` into the text and takes the usage from `response.completed`. A stream that ends before `response.completed`, a `response.failed` and any other end are errors of kind `unreadable`, `rate_limited` (the two usage codes) or `unavailable`, built from the event type and the safe code, never from a body. A `response.incomplete` that says `max_output_tokens` is a finish `length`. `Models` reads `GET {endpoint}/models` as above.
- **The price of a plan model.** The plan has no price per token: its use counts against the limits of the plan, not against a bill. The catalogue of a sign-in connection therefore publishes a price of zero for every model (`price_source` `catalogue`), so a generation through a plan is estimated at zero and is not refused for a missing price, and the ceilings of epic 05 never refuse it. The person can still enter a price in the table, which wins (section 4.2). A zero price is set only by the sign-in adapter, never by a missing entry.
- **Hosts.** A sign-in connection of `openai` has the hosts `api.openai.com` and `auth.openai.com`, so that the refresh passes through the gate of the connection like any request; one of `openrouter` has `openrouter.ai`.

### 12.3 Where a sign-in can be made

The redirect address is a page of this server, `/auth/callback` (SDD-007 section 3.6 item 4), at the address the person opens the application with.

- **OpenRouter** accepts an https address, or `http://localhost:<port>`, so it works on the desktop, on a local server and on a deployment with a public https address.
- **OpenAI** accepts only `http://127.0.0.1:<port>/auth/callback`. A sign-in therefore needs the application opened at `127.0.0.1` on the machine of the person: the desktop application and a local server do; a server on another machine does not, and the begin operation refuses the address with a hint. For such a server the route is to use an API key there; copying a plan credential to a server is a transfer the provider documents as lacking host attribution and revocation, and Stuarty does not offer it. This is a question for the author (`QUESTIONS.md`, "Sign in with ChatGPT on a network deployment").

### 12.4 The operations

Three operations on `service.Service`, in `service/provider_signin_ops.go`, all HTTP only (a person's browser makes a sign-in; there is no command and no tool, SDD-003 section 7.1 exception E-4):

| Operation | Scope | Result |
| --- | --- | --- |
| `ProviderSignInBegin` | admin, and only from a privileged browser session | `connection_id` and `authorize_url`. The request carries `provider`, `label` (optional), `connection_id` (to sign in again an existing connection) and `redirect_uri`. A new connection is made first, with a placeholder credential that holds no token, so the exchange can pass through the gate of that connection; it lists as needing a sign-in until the sign-in ends |
| `ProviderSignInComplete` | admin, and only from a privileged browser session | the sign-in: `connection_id`, `provider`, `label`, `account` (untrusted text) and `state`. The request carries `state`, `code`, `error` and, for OpenAI, `client_id` (the issued id, from the callback) |
| `ProviderSignInList` | read | one entry per connection made by a sign-in: `connection_id`, `provider`, `label`, `account`, `state` (`ready` or `needs_sign_in`); never a token |

`ConnectionRemove` of a connection made by a sign-in is also for a privileged browser session only (an agent cannot remove a sign-in), and `ConnectionCreate` refuses a secret that is the stored form of a sign-in. Begin and complete are audited (`provider.signin.begin`, `provider.signin`), with no token. `E_PROVIDER` gains the code `needs_sign_in` (`details.kind` `auth`): the refresh failed for good and the person must sign in again from the Connections page.

### 12.5 Tests

| What | Test |
| --- | --- |
| The generic flow, the single-use and bound state, the wrong verifier, the expiry, the refresh and its lock | `service/connections/oauth` tests (`TestRefresh*`) |
| The two sign-ins and the adapters over a fake authorization server and recorded responses | `service/providers/openai`, `service/providers/openrouter` tests |
| A sign-in ends with the tokens only in the vault; a replayed or foreign state creates nothing; an agent cannot begin, complete or remove one | `TestSignIn*` in `web/app/servicetest`, `TestSecurity_T75` to `T78` |
| The Connections page with a fake authorization server | `web/e2e/specs/provider-sign-in.spec.ts` |

A real sign-in needs an account of the author: it is the manual check below, and not part of the suite.

**Manual check (the author).** Run `stuarty serve` and open `http://127.0.0.1:7878`. On the Connections page, choose "Sign in with ChatGPT" (a Plus or Pro plan), name the agent on the page of OpenAI, accept, and expect to land on the sign-in page of Stuarty with "signed in". Open the models of the connection and run `stuarty text:generate` with one of them. For OpenRouter choose "Sign in with OpenRouter" and expect a connection whose models list shows prices. Then revoke access in the provider account and expect "needs sign-in" on the card.
