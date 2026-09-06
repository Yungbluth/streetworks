// Category colors come from the paint crews spray on the street before digging

const slug = (name: string) => name.toLowerCase().replace(/[^a-z]+/g, "-").replace(/^-|-$/g, "");

export function paint(category: string): string {
  const css = getComputedStyle(document.documentElement);
  return css.getPropertyValue(`--paint-${slug(category)}`).trim() || css.getPropertyValue("--paint-other").trim();
}

export const cssVar = (name: string) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();

/** white paint needs an outline to show on a light page */
export const needsEdge = (category: string) => slug(category) === "excavation";
