"""Word-timing analysis for whisper.cpp -ojf output.

usage: python -I analyze.py <whisper.json> <silencedetect.log> [use_dtw]
Prints a one-line JSON summary. Times in ms.
"""
import json
import re
import statistics
import sys

SPECIAL = re.compile(r"^\[_.*_\]$")
PUNCT_ONLY = re.compile(r"^[\W_]+$", re.UNICODE)


def load_words(path, use_dtw=False):
    d = json.load(open(path, encoding="utf8"))
    words = []
    n_tokens = 0
    n_punct = 0
    probs = []
    for seg in d["transcription"]:
        text = seg["text"].strip()
        if not text or SPECIAL.match(text):
            continue
        toks = [t for t in seg["tokens"] if not SPECIAL.match(t["text"].strip())]
        n_tokens += len(toks)
        if PUNCT_ONLY.match(text):
            n_punct += 1
        start = seg["offsets"]["from"]
        if use_dtw and toks and toks[0].get("t_dtw", -1) >= 0:
            start = toks[0]["t_dtw"] * 10
        words.append({"text": text, "start": start, "end": seg["offsets"]["to"],
                      "p_min": min(t["p"] for t in toks) if toks else None,
                      "n_tok": len(toks)})
        probs.extend(t["p"] for t in toks)
    return d, words, n_tokens, n_punct, probs


def onsets(path):
    # silence_end = speech onset after a pause
    out = []
    for line in open(path, encoding="utf8", errors="replace"):
        m = re.search(r"silence_end: ([\d.]+)", line)
        if m:
            out.append(float(m.group(1)) * 1000)
    return out


def main():
    sys.stdout.reconfigure(encoding="utf8")
    use_dtw = len(sys.argv) > 3 and sys.argv[3] == "dtw"
    d, words, n_tokens, n_punct, probs = load_words(sys.argv[1], use_dtw)
    ons = onsets(sys.argv[2])
    errs = []
    for o in ons:
        # first word that is still being spoken at or after the detected onset
        cand = [w for w in words if w["end"] > o and not PUNCT_ONLY.match(w["text"])]
        if not cand:
            continue
        errs.append(cand[0]["start"] - o)
    ae = [abs(e) for e in errs]
    res = {
        "entries": len(words),
        "tokens": n_tokens,
        "punct_only_entries": n_punct,
        "multi_token_words": sum(1 for w in words if w["n_tok"] > 1),
        "tokens_p_mean": round(statistics.mean(probs), 3) if probs else None,
        "onsets": len(ons),
        "compared": len(errs),
        "mae_ms": round(statistics.mean(ae)) if ae else None,
        "median_abs_ms": round(statistics.median(ae)) if ae else None,
        "max_abs_ms": round(max(ae)) if ae else None,
        "signed_mean_ms": round(statistics.mean(errs)) if errs else None,
        "within_100ms": sum(1 for a in ae if a <= 100),
        "within_200ms": sum(1 for a in ae if a <= 200),
        "within_300ms": sum(1 for a in ae if a <= 300),
        "lang": d["result"]["language"],
        "text40": " ".join(w["text"] for w in words[:40]),
    }
    print(json.dumps(res, ensure_ascii=False))


main()
