// Locates values found by the Go detector in the PDF.js text layer.
// This is matching only: the preview has no PII rules of its own.

/** Lowercases a word and drops punctuation that PDF.js and Go split differently. */
export function cleanWord(w: string): string {
  return w.trim().replace(/[.,:;()\[\]"'\-–—]/g, '').toLowerCase();
}

const isDigit = (c: string | undefined): boolean => c !== undefined && c >= '0' && c <= '9';

/**
 * Finds every run of tokens whose cleaned text, glued without spaces, equals the
 * cleaned target. Spacing is ignored on purpose: PDF.js splits a word into
 * several items when the font changes inside it ("Сан" + "кт" + "-" +
 * "Петербург"), while Go may insert a space at a line wrap ("Сан кт-Петербург").
 *
 * A run must start and end on token boundaries, so "Иван" never matches inside
 * "Иванов". A number may also sit inside a longer token ("тел89161234567") as
 * long as it does not cut a digit sequence.
 *
 * Returns [firstTokenIndex, lastTokenIndex] pairs, inclusive.
 */
export function findTokenRuns(tokens: ReadonlyArray<{ clean: string }>, target: string): Array<[number, number]> {
  const needle = target.trim().split(/\s+/).map(cleanWord).join('');
  if (!needle) return [];

  let hay = '';
  const tokenStart: number[] = [];
  const charToken: number[] = [];
  tokens.forEach((t, idx) => {
    tokenStart.push(hay.length);
    for (let k = 0; k < t.clean.length; k++) charToken.push(idx);
    hay += t.clean;
  });

  const runs: Array<[number, number]> = [];
  let from = 0;
  while (from <= hay.length - needle.length) {
    const at = hay.indexOf(needle, from);
    if (at === -1) break;
    const end = at + needle.length; // exclusive
    const first = charToken[at]!;
    const last = charToken[end - 1]!;

    const startsOnBoundary = tokenStart[first] === at;
    const endsOnBoundary = last + 1 >= tokens.length ? end === hay.length : tokenStart[last + 1] === end;
    const startOk = startsOnBoundary || (isDigit(needle[0]) && !isDigit(hay[at - 1]));
    const endOk = endsOnBoundary || (isDigit(needle[needle.length - 1]) && !isDigit(hay[end]));

    if (startOk && endOk) {
      runs.push([first, last]);
      from = end;
    } else {
      from = at + 1;
    }
  }
  return runs;
}
