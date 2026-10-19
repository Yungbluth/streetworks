import { h } from "../dom";
import { placeName, plural } from "../format";
import { permitsMatching } from "../filters";
import { distanceMeters, type LatLng } from "../geo";
import { byDateDesc, detailsFor, permitAt, type Ctx } from "../model";
import { RADII } from "../url";
import { failed, kindSplit, loading, tableStyle } from "./common";
import { permitTable } from "./table";

export function nearby(ctx: Ctx, center: LatLng, opts: { listNow?: boolean; exclude?: number } = {}): HTMLElement {
  const { site } = ctx;
  const found = permitsMatching(site.permits, ctx.filter, { center, radiusM: ctx.radiusM }).filter(
    (i) => site.permits.place[i] !== opts.exclude,
  );
  const placesHit = new Set(found.map((i) => site.permits.place[i]!));
  const cats = new Map<number, number>();
  for (const i of found) cats.set(site.permits.cat[i]!, (cats.get(site.permits.cat[i]!) ?? 0) + 1);

  const select = h(
    "select",
    { "aria-label": "Within", onchange: (e) => ctx.setRadius(Number((e.target as HTMLSelectElement).value)) },
    ...RADII.map((r) => h("option", { value: r.m, selected: r.m === ctx.radiusM }, r.label)),
  );
  const out = h(
    "section",
    { class: "nearby" },
    h("h3", {}, "Within ", select),
    h(
      "p",
      { class: "sub" },
      !found.length
        ? opts.exclude === undefined
          ? "No permits match the filters inside this circle."
          : "No other permits match the filters inside this circle."
        : opts.exclude === undefined
          ? `${plural(found.length, "permit")} at ${plural(placesHit.size, "place")}`
          : `${plural(found.length, "more permit")} at ${plural(placesHit.size, "other place")}`,
    ),
  );
  if (!found.length) return out;
  if (opts.exclude === undefined) out.append(kindSplit(cats, site.index.categories));

  const list = h("div", {});
  let focusList = false;
  const show = async () => {
    list.replaceChildren(loading());
    try {
      const details = await detailsFor(site, found);
      const entries = found
        .map((i) => {
          const place = site.permits.place[i]!;
          return {
            index: i,
            category: site.index.categories[site.permits.cat[i]!]!,
            detail: details.get(i),
            distanceM: distanceMeters(center, permitAt(site, i)),
            placeName: placeName(site.places, place),
            onPlace: () => ctx.go({ kind: "place", place }, { move: true }),
          };
        })
        .sort((a, b) => byDateDesc(a.detail, b.detail) || a.distanceM - b.distanceM);
      list.replaceChildren(permitTable(entries, tableStyle("Away")));
      if (focusList) list.querySelector<HTMLElement>(".permit-open")?.focus();
    } catch (e) {
      list.replaceChildren(failed("the permits here", e, () => void show()));
    }
  };
  if (opts.listNow) void show();
  else
    list.append(
      h("button", { type: "button", class: "more", onclick: () => ((focusList = true), void show()) }, `List them, newest first`),
    );
  out.append(list);
  return out;
}
