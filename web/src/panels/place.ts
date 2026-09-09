import { h } from "../dom";
import { placeName, plural, yearsText } from "../format";
import { byDateDesc, detailsFor, permitsOf, placeAt, type Ctx } from "../model";
import { failed, loading, note, panelHead } from "./common";
import { historyList } from "./history";
import { nearby } from "./radius";

export function placePanel(ctx: Ctx, place: number, extraNote?: string): HTMLElement {
  const { site, filter } = ctx;
  const mine = permitsOf(site, place, filter);
  const years = mine.map((i) => site.permits.year[i]!).filter(Boolean);
  const sub = mine.length
    ? `${plural(mine.length, "permit")} here${years.length ? `, ${yearsText(Math.min(...years), Math.max(...years))}` : ""}`
    : "No permits here match the filters.";

  const history = h("div", {}, loading());
  const out = h(
    "section",
    { class: "focus" },
    panelHead(placeName(site.places, place), () => ctx.go(null)),
    note(extraNote),
    h("p", { class: "sub" }, sub),
    history,
    nearby(ctx, placeAt(site, place)),
  );
  if (!mine.length) history.replaceChildren();
  else
    detailsFor(site, mine)
      .then((details) => {
        const entries = mine
          .map((i) => ({ index: i, category: site.index.categories[site.permits.cat[i]!]!, detail: details.get(i) }))
          .sort((a, b) => byDateDesc(a.detail, b.detail));
        history.replaceChildren(historyList(entries));
      })
      .catch((e: unknown) => history.replaceChildren(failed("this place's permits", e)));
  return out;
}

/** a point with no place of its own, such as where two streets come closest */
export function spotPanel(ctx: Ctx, title: string, at: { lat: number; lng: number }, extraNote?: string): HTMLElement {
  return h("section", { class: "focus" }, panelHead(title, () => ctx.go(null)), note(extraNote), nearby(ctx, at, { listNow: true }));
}
