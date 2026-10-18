// Category colors come from the paint crews spray on the street before digging

const slug = (name: string) => name.toLowerCase().replace(/[^a-z]+/g, "-").replace(/^-|-$/g, "");

export const cssVar = (name: string) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();

export function paint(category: string): string {
  return cssVar(`--paint-${slug(category)}`) || cssVar("--paint-other");
}

/** white paint needs an outline to show on a light page */
export const needsEdge = (category: string) => slug(category) === "excavation";

/** The outline a kind's dots get, if any */
/** a kind's halo, if the stylesheet gives it one (--paint-excavation-halo) */
export function haloFor(category: string): string | undefined {
  const own = cssVar(`--paint-${slug(category)}-halo`);
  return own && own !== "none" ? own : undefined;
}

export function edgeFor(category: string): string | undefined {
  const own = cssVar(`--paint-${slug(category)}-edge`);
  if (own === "none") return undefined;
  if (own) return own;
  return needsEdge(category) ? cssVar("--paint-edge") : undefined;
}
