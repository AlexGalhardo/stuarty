"""Tiny-sample word error count over the first 40 reference words.

usage: python -I wer40.py <whisper.json> <reference.txt> <start phrase> [n=40]
Normalisation: lowercase, accents folded, punctuation removed. Prints the alignment ops
(S=substitution, D=deletion of a reference word, I=insertion) so orthographic variants
(old Portuguese spelling in the reference) can be told apart from real recognition errors.
"""
import json
import re
import sys
import unicodedata

sys.stdout.reconfigure(encoding="utf8")


def norm(w):
    w = unicodedata.normalize("NFKD", w.lower())
    return re.sub(r"[\W_]+", "", "".join(c for c in w if not unicodedata.combining(c)))


def main():
    jpath, rpath, start = sys.argv[1], sys.argv[2], sys.argv[3]
    n = int(sys.argv[4]) if len(sys.argv) > 4 else 40
    text = open(rpath, encoding="utf8").read()
    i = text.lower().replace("\r", "").find(start.lower())
    ref = [norm(w) for w in text[i:i + 2500].split()]
    ref = [w for w in ref if w][:n]
    d = json.load(open(jpath, encoding="utf8"))
    hyp = [norm(s["text"]) for s in d["transcription"] if norm(s["text"])]
    hyp = [w for w in hyp][: n + 6]
    # Levenshtein with free tail on the hypothesis (hyp may run past the 40th ref word)
    R, H = len(ref), len(hyp)
    D = [[0] * (H + 1) for _ in range(R + 1)]
    for r in range(R + 1):
        D[r][0] = r
    for h in range(H + 1):
        D[0][h] = h
    for r in range(1, R + 1):
        for h in range(1, H + 1):
            D[r][h] = min(D[r - 1][h] + 1, D[r][h - 1] + 1, D[r - 1][h - 1] + (ref[r - 1] != hyp[h - 1]))
    best = min(range(H + 1), key=lambda h: D[R][h])
    r, h, ops = R, best, []
    while r > 0 or h > 0:
        if r and h and D[r][h] == D[r - 1][h - 1] + (ref[r - 1] != hyp[h - 1]):
            if ref[r - 1] != hyp[h - 1]:
                ops.append(f"S {ref[r-1]}->{hyp[h-1]}")
            r, h = r - 1, h - 1
        elif r and D[r][h] == D[r - 1][h] + 1:
            ops.append(f"D {ref[r-1]}")
            r -= 1
        else:
            ops.append(f"I {hyp[h-1]}")
            h -= 1
    print(json.dumps({"ref_words": R, "errors": D[R][best], "ops": ops[::-1]}, ensure_ascii=False))


main()
