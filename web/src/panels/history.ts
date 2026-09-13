import type { Detail } from "../data";
import { h } from "../dom";
import { dateText, feet, num } from "../format";
import { paint } from "../paint";

export interface Entry {
  index: number;
  category: string;
  detail?: Detail;
  distanceM?: number;
  placeName?: string;
  onPlace?: () => void;
}

const PAGE = 25;

// which date field a permit is listed under, when it isn't the issue date
const dateBases: Record<string, string> = {
  DATE_APPROVE: "Dated by approval",
  DATE_START: "Dated by the start of work",
  DATE_ENTERED: "Dated by when it was entered",
};

function item(e: Entry): HTMLLIElement {
  const d = e.detail;
  const what = d ? d.description || d.subType || d.type : "Details for this permit didn't load.";
  const who = d ? [d.applicant, d.utility].filter(Boolean).join(", ") : "";
  return h(
    "li",
    { class: d?.noWork ? "permit no-work" : "permit" },
    h("span", { class: "date" }, d ? dateText(d.date) : ""),
    h(
      "div",
      {},
      h("p", { class: "what" }, what),
      h(
        "p",
        { class: "meta" },
        h("span", { class: "swatch", style: `--swatch: ${paint(e.category)}`, "aria-hidden": "true" }),
        d ? `${e.category}, ${d.status.toLowerCase()}` : e.category,
        d?.noWork ? h("span", { class: "tag" }, "no work done") : null,
      ),
      who ? h("p", { class: "who" }, who) : null,
      d
        ? h(
            "p",
            { class: "filed" },
            d.filed ? `Filed as ${d.filed}` : null,
            d.final ? h("span", {}, `Finished ${dateText(d.final)}`) : null,
            dateBases[d.dateBasis] ? h("span", {}, dateBases[d.dateBasis]) : null,
          )
        : null,
      d
        ? h(
            "p",
            { class: "ids" },
            d.permit,
            d.link ? h("a", { href: d.link, target: "_blank", rel: "noopener" }, "City record") : null,
          )
        : null,
      e.distanceM !== undefined
        ? h(
            "p",
            { class: "where" },
            `${feet(e.distanceM)} away`,
            e.onPlace && e.placeName ? h("button", { type: "button", class: "linkish", onclick: e.onPlace }, e.placeName) : null,
          )
        : null,
      ...(d?.flags ?? []).map((f) => h("p", { class: "flag" }, f)),
    ),
  );
}

export function historyList(entries: Entry[]): HTMLElement {
  const list = h("ol", { class: "history" });
  const wrap = h("div", {}, list);
  let shown = 0;
  const more = h("button", { type: "button", class: "more" });
  const page = () => {
    list.append(...entries.slice(shown, shown + PAGE).map(item));
    shown = Math.min(entries.length, shown + PAGE);
    more.textContent = `Show ${num(Math.min(PAGE, entries.length - shown))} more of ${num(entries.length - shown)}`;
    if (shown >= entries.length) more.remove();
  };
  more.addEventListener("click", page);
  wrap.append(more);
  page();
  return wrap;
}
