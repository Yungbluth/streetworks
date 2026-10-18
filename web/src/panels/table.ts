import type { Detail } from "../data";
import { h } from "../dom";
import { dateText, feet, num } from "../format";
import { strokeIcon, swatch } from "../marks";

export interface Entry {
  index: number;
  category: string;
  detail?: Detail;
  /** Its number at its place, counting up from the oldest permit */
  delta?: number;
  distanceM?: number;
  placeName?: string;
  onPlace?: () => void;
}

export interface TableStyle {
  /** what the first column holds: revision numbers, distance, or nothing */
  first: "Δ" | "Away" | null;
  button: string;
  link: string;
}

const PAGE = 25;

// which date field a permit is listed under, when it isn't the issue date
const dateBases: Record<string, string> = {
  DATE_APPROVE: "Dated by approval",
  DATE_START: "Dated by the start of work",
  DATE_ENTERED: "Dated by when it was entered",
};

let rowId = 0;

function rows(e: Entry, style: TableStyle): HTMLTableRowElement[] {
  const d = e.detail;
  const what = d ? d.description || d.subType || d.type : "Details for this permit didn't load.";
  const who = d ? [d.applicant, d.utility].filter(Boolean).join(", ") : "";
  const span = style.first ? 3 : 2;
  const id = `permit-${++rowId}`;
  const more = h(
    "tr",
    { class: "permit-more", id, hidden: true },
    h(
      "td",
      { colspan: span },
      h(
        "dl",
        {},
        h("dt", {}, "Kind"), h("dd", {}, d ? `${e.category}, ${d.status.toLowerCase()}` : e.category),
        ...(who ? [h("dt", {}, "Applicant"), h("dd", {}, who)] : []),
        ...(d?.filed ? [h("dt", {}, "Filed as"), h("dd", {}, d.filed)] : []),
        ...(d?.final ? [h("dt", {}, "Finished"), h("dd", {}, dateText(d.final))] : []),
        ...(e.onPlace && e.placeName ? [h("dt", {}, "At"), h("dd", {}, h("button", { type: "button", class: style.link, onclick: e.onPlace }, e.placeName))] : []),
        ...(d
          ? [
              h("dt", {}, "Permit"),
              h(
                "dd",
                { class: "ids" },
                h("span", { class: "permit-no" }, d.permit),
                d.link ? h("a", { href: d.link, target: "_blank", rel: "noopener", "aria-label": `City record for ${d.permit}` }, "City record") : null,
              ),
            ]
          : []),
      ),
      d && dateBases[d.dateBasis] ? h("p", { class: "quiet" }, dateBases[d.dateBasis]) : null,
      ...(d?.flags ?? []).map((f) => h("p", { class: "flag" }, f)),
    ),
  );
  const toggle = h(
    "button",
    {
      type: "button", class: "permit-open", "aria-expanded": "false", "aria-controls": id,
      onclick: () => {
        const open = toggle.getAttribute("aria-expanded") !== "true";
        toggle.setAttribute("aria-expanded", String(open));
        more.hidden = !open;
        toggle.querySelector("svg.plus, svg.minus")?.replaceWith(strokeIcon(open ? "minus" : "plus"));
        row.classList.toggle("open", open);
      },
    },
    swatch(e.category, 10),
    h("span", { class: "permit-what" }, what),
    // The swatch is colour alone, so the kind is written out too
    h("span", { class: "permit-kind" }, e.category, d?.noWork ? ", no work done" : "", style.first === "Away" && e.placeName ? ` · ${e.placeName}` : ""),
    // a plus that turns to a minus, so a line shows that it opens
    strokeIcon("plus"),
  );
  const first =
    style.first === "Away" ? (e.distanceM === undefined ? "" : feet(e.distanceM)) : style.first === "Δ" && e.delta ? `Δ${e.delta}` : "";
  const row = h(
    "tr",
    { class: d?.noWork ? "permit-row no-work" : "permit-row" },
    style.first ? h("td", { class: "permit-first" }, first) : null,
    h("td", { class: "permit-date" }, d ? dateText(d.date) : ""),
    h("td", { class: "permit-work" }, toggle),
  );
  return [row, more];
}

export function permitTable(entries: Entry[], style: TableStyle): HTMLElement {
  const body = h("tbody", {});
  const table = h(
    "table",
    { class: "permit-table" },
    h(
      "thead",
      {},
      h(
        "tr",
        {},
        style.first ? h("th", { scope: "col", class: "permit-first" }, style.first) : null,
        h("th", { scope: "col", class: "permit-date" }, "Date"),
        h("th", { scope: "col" }, "Work"),
      ),
    ),
    body,
  );
  const wrap = h("div", { class: "permit-wrap" }, table);
  let shown = 0;
  const more = h("button", { type: "button", class: style.button });
  const page = () => {
    const added = entries.slice(shown, shown + PAGE).flatMap((e) => rows(e, style));
    body.append(...added);
    shown = Math.min(entries.length, shown + PAGE);
    const left = entries.length - shown;
    more.textContent = left === 1 ? "Show the last one" : left <= PAGE ? `Show the last ${num(left)}` : `Show ${PAGE} more of ${num(left)}`;
    if (shown >= entries.length) more.remove();
    return added[0];
  };
  more.addEventListener("click", () => page()?.querySelector<HTMLElement>(".permit-open")?.focus());
  wrap.append(more);
  page();
  return wrap;
}
