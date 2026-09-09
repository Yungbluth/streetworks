import { h } from "../dom";
import { placeName, plural } from "../format";
import { permitsMatching } from "../filters";
import { distanceMeters, type LatLng } from "../geo";
import { byDateDesc, detailsFor, permitAt, type Ctx } from "../model";
import { RADII } from "../url";
import { categoryBars, failed, loading } from "./common";
import { historyList } from "./history";

export function nearby(ctx: Ctx, center: LatLng, opts: { listNow?: boolean } = {}): HTMLElement {
  const { site } = ctx;
  const found = permitsMatching(site.permits, ctx.filter, { center, radiusM: ctx.radiusM });
  const placesHit = new Set(found.map((i) => site.permits.place[i]!));
  const cats = new Map<number, number>();
  for (const i of found) cats.set(site.permits.cat[i]!, (cats.get(site.permits.cat[i]!) ?? 0) + 1);

  const select = h(
    "select",
    { "aria-label": "Distance", onchange: (e) => ctx.setRadius(Number((e.target as HTMLSelectElement).value)) },
    ...RADII.map((r) => h("option", { value: r.m, selected: r.m === ctx.radiusM }, r.label)),
  );
  const out = h(
    "section",
    { class: "nearby" },
    h("h3", {}, "Within ", select),
    h("p", { class: "sub" }, found.length ? `${plural(found.length, "permit")} at ${plural(placesHit.size, "place")}` : "No permits match the filters inside this circle."),
  );
  if (!found.length) return out;
  out.append(categoryBars(cats, site.index.categories));

  const list = h("div", {});
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
      list.replaceChildren(historyList(entries));
    } catch (e) {
      list.replaceChildren(failed("the permits here", e));
    }
  };
  if (opts.listNow) void show();
  else
    list.append(
      h("button", { type: "button", class: "more", onclick: () => void show() }, `List all ${plural(found.length, "permit")}, newest first`),
    );
  out.append(list);
  return out;
}
