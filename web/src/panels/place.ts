import { PlaceKind, type Detail } from "../data";
import { h } from "../dom";
import { num, placeName, plural, prettyStreet, yearsText } from "../format";
import { byDateDesc, detailsFor, digKinds, permitsOf, placeAt, type Ctx } from "../model";
import { failed, kindSplit, loading, note, panelHead, showAll, tableStyle } from "./common";
import { nearby } from "./radius";
import { streetBlocks } from "./street";
import { permitTable } from "./table";

/** The answer to "what's been happening here?" in one sentence */
export function placeSummary(byCategory: Map<number, number>, years: number[], categories: string[]): string {
  const total = [...byCategory.values()].reduce((a, b) => a + b, 0);
  if (!total) return "No permits here match the filters.";
  const dated = years.filter(Boolean);
  const lo = Math.min(...dated);
  const hi = Math.max(...dated);
  const when = !dated.length ? "" : lo === hi ? ` in ${lo}` : ` from ${lo} to ${hi}`;
  const top = [...byCategory].sort((a, b) => b[1] - a[1]);
  const name = (c: number) => categories[c] ?? "Other";
  let kinds: string;
  if (total === 1) kinds = name(top[0]![0]);
  else if (top.length === 1) kinds = `all ${name(top[0]![0])}`;
  else kinds = `mostly ${name(top[0]![0])} (${top[0]![1]}) and ${name(top[1]![0])} (${top[1]![1]})`;
  return `${plural(total, "permit")}${when}, ${kinds}.`;
}

const median = (xs: number[]) => {
  const s = [...xs].sort((a, b) => a - b);
  const mid = s.length >> 1;
  return s.length % 2 ? s[mid]! : (s[mid - 1]! + s[mid]!) / 2;
};

const times = (n: number) => (n === 1 ? "once" : n === 2 ? "twice" : `${num(n)} times`);

export function digSummary(years: number[], street: { typical: number; street: string } | undefined): string {
  if (!years.length) return "Not dug up in these years.";
  const n = years.length;
  const last = Math.max(...years);
  const head = n === 1 ? `Dug up once, in ${last}` : `Dug up ${times(n)}, most recently in ${last}`;
  if (!street) return `${head}.`;
  const shown = Math.round(street.typical);
  if (street.typical === 0) return `${head}; most blocks on ${street.street} haven't been dug up at all.`;
  const r = n / street.typical;
  const than = `the typical block on ${street.street} (${plural(shown, "dig")})`;
  if (r >= 1.5) {
    const ratio = r < 2.5 ? "about twice" : r < 3.5 ? "about three times" : `about ${Math.round(r)} times`;
    return `${head}, ${ratio} ${than}.`;
  }
  if (r <= 2 / 3) return `${head}, fewer than ${than}.`;
  return `${head}, about the same as ${than}.`;
}

export function whoFor(details: Detail[]): string | null {
  const counts = new Map<string, number>();
  for (const d of details) {
    const name = d.utility?.trim() || d.applicant?.trim();
    if (name) counts.set(name, (counts.get(name) ?? 0) + 1);
  }
  const top = [...counts].sort((a, b) => b[1] - a[1]);
  if (!top.length) return null;
  if (top.length === 1) return top[0]![1] === 1 ? `Filed for ${top[0]![0]}.` : `All filed for ${top[0]![0]}.`;
  return `Filed mostly for ${top[0]![0]} (${num(top[0]![1])}) and ${top[1]![0]} (${num(top[1]![1])}).`;
}

export function perYear(years: number[], from: number, to: number): number[] {
  const out = new Array<number>(to - from + 1).fill(0);
  for (const y of years) if (y >= from && y <= to) out[y - from]!++;
  return out;
}

/** A place's permits per year as a small strip of bars over the chosen years */
function placeYears(years: number[], from: number, to: number): HTMLElement {
  const counts = perYear(years, from, to);
  const max = Math.max(1, ...counts);
  const busy = counts.flatMap((n, i) => (n ? [`${num(n)} in ${from + i}`] : []));
  return h(
    "figure",
    { class: "place-years", role: "img", "aria-label": `Permits per year, ${yearsText(from, to)}: ${busy.join(", ")}.` },
    h("div", { class: "bars" }, ...counts.map((n, i) => h("span", { class: n ? "bar" : "bar none", style: `--h: ${(n / max) * 100}%`, title: `${from + i}: ${plural(n, "permit")}` }))),
    h("figcaption", { class: "ends", "aria-hidden": "true" }, h("span", {}, String(from)), h("span", {}, String(to))),
  );
}

export function placePanel(ctx: Ctx, place: number, extraNote?: string, back?: { label: string; go: () => void }): HTMLElement {
  const { site, filter } = ctx;
  const mine = permitsOf(site, place, filter);
  const byCategory = new Map<number, number>();
  for (const i of mine) byCategory.set(site.permits.cat[i]!, (byCategory.get(site.permits.cat[i]!) ?? 0) + 1);

  const digCats = digKinds(site, filter);
  const digYears = (p: number) => permitsOf(site, p, filter).filter((i) => digCats.has(site.permits.cat[i]!)).map((i) => site.permits.year[i]!);
  const isBlock = site.places.kind[place] === PlaceKind.Block;
  const neighbours = isBlock ? streetBlocks(ctx, site.places.street[place]!).filter((p) => permitsOf(site, p, filter).length) : [];
  const typical = neighbours.length > 1 ? median(neighbours.map((p) => digYears(p).length)) : undefined;
  const digs = digCats.size
    ? digSummary(digYears(place), typical === undefined ? undefined : { typical, street: prettyStreet(site.places.streets[site.places.street[place]!]!) })
    : null;

  const history = h("div", { class: "history" }, mine.length ? loading() : null);
  const who = h("p", { class: "who", hidden: true });
  const out = h(
    "section",
    { class: "focus" },
    back ? h("button", { type: "button", class: "linkish back-to", onclick: back.go }, `Back to ${back.label}`) : null,
    panelHead(placeName(site.places, place), () => ctx.go(null)),
    note(extraNote),
    digs ? h("p", { class: "digs" }, digs) : null,
    digs ? h("p", { class: "quiet digs-note" }, "A dig is an Excavation or Combo permit.") : null,
    h("p", { class: "lead" }, placeSummary(byCategory, mine.map((i) => site.permits.year[i]!), site.index.categories)),
    who,
    byCategory.size > 1 ? kindSplit(byCategory, site.index.categories) : null,
    mine.length > 1 ? placeYears(mine.map((i) => site.permits.year[i]!), filter.from, filter.to) : null,
    !mine.length && filter.cats.size < site.index.categories.length ? showAll(ctx.showAllKinds) : null,
    // What's around it comes before the full history, which can run long
    nearby(ctx, placeAt(site, place), { exclude: place }),
    mine.length ? h("h3", {}, "Permits here, newest first") : null,
    history,
  );
  if (mine.length)
    detailsFor(site, mine)
      .then((details) => {
        const entries = mine
          .map((i) => ({ index: i, category: site.index.categories[site.permits.cat[i]!]!, detail: details.get(i) }))
          .sort((a, b) => byDateDesc(a.detail, b.detail));
        history.replaceWith(permitTable(entries, tableStyle(null)));
        const named = whoFor(entries.flatMap((e) => (e.detail ? [e.detail] : [])));
        if (named) {
          who.textContent = named;
          who.hidden = false;
        }
      })
      .catch((e: unknown) => history.replaceChildren(failed("this place's permits", e, ctx.retry)));
  return out;
}

/** a point with no place of its own, such as where two streets come closest */
export function spotPanel(ctx: Ctx, title: string, at: { lat: number; lng: number }, extraNote?: string): HTMLElement {
  return h("section", { class: "focus" }, panelHead(title, () => ctx.go(null)), note(extraNote), nearby(ctx, at, { listNow: true }));
}
