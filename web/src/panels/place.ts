import { h } from "../dom";
import { placeName, plural } from "../format";
import { byDateDesc, detailsFor, permitsOf, placeAt, type Ctx } from "../model";
import { failed, kindSplit, loading, note, panelHead, showAll, tableStyle } from "./common";
import { nearby } from "./radius";
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

export function placePanel(ctx: Ctx, place: number, extraNote?: string, back?: { label: string; go: () => void }): HTMLElement {
  const { site, filter } = ctx;
  const mine = permitsOf(site, place, filter);
  const byCategory = new Map<number, number>();
  for (const i of mine) byCategory.set(site.permits.cat[i]!, (byCategory.get(site.permits.cat[i]!) ?? 0) + 1);

  const history = h("div", { class: "history" }, mine.length ? loading() : null);
  const out = h(
    "section",
    { class: "focus" },
    back ? h("button", { type: "button", class: "linkish back", onclick: back.go }, `Back to ${back.label}`) : null,
    panelHead(placeName(site.places, place), () => ctx.go(null)),
    note(extraNote),
    h("p", { class: "lead" }, placeSummary(byCategory, mine.map((i) => site.permits.year[i]!), site.index.categories)),
    byCategory.size > 1 ? kindSplit(byCategory, site.index.categories) : null,
    !mine.length && filter.cats.size < site.index.categories.length ? showAll(ctx.showAllKinds) : null,
    mine.length ? h("h3", {}, "Permits here, newest first") : null,
    history,
    nearby(ctx, placeAt(site, place), { exclude: place }),
  );
  if (mine.length)
    detailsFor(site, mine)
      .then((details) => {
        const entries = mine
          .map((i) => ({ index: i, category: site.index.categories[site.permits.cat[i]!]!, detail: details.get(i) }))
          .sort((a, b) => byDateDesc(a.detail, b.detail));
        history.replaceWith(permitTable(entries, tableStyle(null)));
      })
      .catch((e: unknown) => history.replaceChildren(failed("this place's permits", e, ctx.retry)));
  return out;
}

/** a point with no place of its own, such as where two streets come closest */
export function spotPanel(ctx: Ctx, title: string, at: { lat: number; lng: number }, extraNote?: string): HTMLElement {
  return h("section", { class: "focus" }, panelHead(title, () => ctx.go(null)), note(extraNote), nearby(ctx, at, { listNow: true }));
}
