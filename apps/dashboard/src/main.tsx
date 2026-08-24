import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { stateMessage } from "./state";
import "./style.css";

function App() {
  return <main><p className="eyebrow">STICKGUY</p><h1>Coordination, with evidence.</h1><p>{stateMessage("loading")}</p></main>;
}

createRoot(document.getElementById("root")!).render(<StrictMode><App /></StrictMode>);
