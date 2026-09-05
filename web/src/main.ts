import { loadSite } from "./data";

loadSite(new URL("data/", document.baseURI).href).then((site) => {
  document.getElementById("app")!.textContent = `${site.permits.place.length} permits`;
});
