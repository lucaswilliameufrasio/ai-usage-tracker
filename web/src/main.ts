import "./styles.css";
import { start } from "./app.ts";

const root = document.querySelector("#app");
if (!(root instanceof HTMLElement)) {
  throw new Error("raiz ausente");
}
start(root);
