# Generates the labelled speech fixture of epic 69 (Windows only: System.Speech).
#
#   powershell -NoProfile -ExecutionPolicy Bypass -File gen-speech.ps1 -Lang en -Out C:\scratch
#
# Writes <Out>\<Lang>.wav (16 kHz, mono, 16 bit) and <Out>\<Lang>.labels.json: every spoken word with its
# kind and the onset the synthesizer reported (SpeakProgress). Kinds: "word", "filler", "repeat" (the first
# copy of a repeated sequence, the one a person would remove). The audio is synthetic: it measures the
# detector and the pinned whisper.cpp on spoken fillers it is told about, not on human hesitation.
param(
	[Parameter(Mandatory = $true)][ValidateSet('en', 'pt')][string]$Lang,
	[Parameter(Mandatory = $true)][string]$Out
)

Add-Type -AssemblyName System.Speech

# {filler}  one filler word; {you_know} a filler of two words; [first copy] words that are repeated right after.
$scripts = @{
	en = @(
		'So {um} today I want to talk about {uh} how we plan a video.',
		'First [we should] we should decide the topic, {you_know} the one people ask about.',
		'Then {um} we write the script and {uh} record it in one take.',
		'[The] the camera should be at eye level, and {er} the light should come from the front.',
		'I [think that] I think that sound matters more than {um} the picture.',
		'After recording, {uh} we cut the pauses and {you_know} keep the good parts.',
		'[It is] it is faster than {um} scrubbing the timeline by hand.',
		'Finally {hmm} we export the file and share it with everyone.',
		'Some people {uh} start with the title, but I [prefer to] I prefer to start with the story.',
		'The first ten seconds {um} decide whether anyone keeps watching.',
		'So [keep it] keep it short, {er} clear and honest.',
		'Then {hmm} read the captions once {uh} before you publish.'
	)
	pt = @(
		'Então {ééé} hoje eu quero falar sobre {hum} como planejar um vídeo.',
		'Primeiro [a gente vai] a gente vai escolher o tema, {né} o assunto que o pessoal pergunta.',
		'Depois {ahn} a gente escreve o roteiro e {éé} grava de uma vez.',
		'[A] a câmera tem que ficar na altura dos olhos, e {hum} a luz vem de frente.',
		'Eu [acho que] eu acho que o som importa mais do que {ééé} a imagem.',
		'Depois de gravar, {ahn} a gente corta as pausas e {tipo} guarda as partes boas.',
		'[É mais] é mais rápido do que {hum} arrastar a linha do tempo na mão.',
		'No fim {hum} a gente exporta o arquivo e compartilha com todo mundo.',
		'Tem gente que {ahn} começa pelo título, mas eu [prefiro começar] prefiro começar pela história.',
		'Os primeiros dez segundos {hum} decidem se alguém continua assistindo.',
		'Então [seja curto] seja curto, {éé} claro e honesto.',
		'Depois {hum} leia as legendas uma vez {ahn} antes de publicar.'
	)
}
$voice = @{ en = 'Microsoft Zira Desktop'; pt = 'Microsoft Maria Desktop' }[$Lang]

$tokens = New-Object System.Collections.Generic.List[object]
$text = New-Object System.Text.StringBuilder
foreach ($line in $scripts[$Lang]) {
	$inRepeat = $false
	foreach ($raw in ($line -split ' ')) {
		$t = $raw
		$thisKind = 'word'
		if ($t.StartsWith('{')) {
			$thisKind = 'filler'
			$t = $t.Trim('{', '}')
		}
		else {
			if ($t.StartsWith('[')) { $inRepeat = $true; $t = $t.TrimStart('[') }
			if ($inRepeat) { $thisKind = 'repeat' }
			if ($t.EndsWith(']')) { $t = $t.TrimEnd(']'); $inRepeat = $false }
		}
		foreach ($piece in ($t -split '_')) {
			$norm = ($piece.ToLowerInvariant() -replace '[\p{P}\p{S}]', '')
			$start = $text.Length
			[void]$text.Append($piece + ' ')
			$tokens.Add([pscustomobject]@{ text = $norm; kind = $thisKind; char = $start; len = $piece.Length; start_ms = -1 })
		}
	}
}

$synth = New-Object System.Speech.Synthesis.SpeechSynthesizer
$synth.SelectVoice($voice)
$synth.Rate = 0
New-Item -ItemType Directory -Force -Path $Out | Out-Null
$wav = Join-Path $Out "$Lang.wav"
$fmt = New-Object System.Speech.AudioFormat.SpeechAudioFormatInfo(16000, [System.Speech.AudioFormat.AudioBitsPerSample]::Sixteen, [System.Speech.AudioFormat.AudioChannel]::Mono)
$synth.SetOutputToWaveFile($wav, $fmt)
$events = New-Object System.Collections.Generic.List[object]
$synth.add_SpeakProgress({ param($sender, $e) $script:events.Add([pscustomobject]@{ char = $e.CharacterPosition; ms = [int]$e.AudioPosition.TotalMilliseconds }) })
$synth.Speak($text.ToString())
$synth.Dispose()

foreach ($e in $events) {
	foreach ($tk in $tokens) { if ($tk.char -eq $e.char -and $tk.start_ms -lt 0) { $tk.start_ms = $e.ms } }
}
$words = $tokens | ForEach-Object { [ordered]@{ text = $_.text; kind = $_.kind; start_ms = $_.start_ms } }
$json = [ordered]@{ lang = $Lang; voice = $voice; words = @($words) } | ConvertTo-Json -Depth 5
[System.IO.File]::WriteAllText((Join-Path $Out "$Lang.labels.json"), $json, (New-Object System.Text.UTF8Encoding($false)))
"words=$($tokens.Count) events=$($events.Count) wav=$wav"
