import React from "react";
import { createRoot } from "react-dom/client";
import "./viewer.css";

function BootstrapError({ message }: { message: string }) {
  return (
    <main className="bootstrap-error">
      <h1>Semantic Changes</h1>
      <p>{message}</p>
    </main>
  );
}

function start() {
  const root = document.getElementById("root");
  if (!root) return;
  const data = document.getElementById("semdiff-data");
  if (!data?.textContent) {
    createRoot(root).render(<BootstrapError message="Viewer data is missing." />);
    return;
  }
  try {
    const page = JSON.parse(data.textContent) as { base_sha?: string; head_sha?: string };
    createRoot(root).render(
      <main className="bootstrap-error">
        <h1>Semantic Changes</h1>
        <p>
          {page.base_sha ?? "unknown"} → {page.head_sha ?? "unknown"}
        </p>
      </main>,
    );
  } catch (error) {
    createRoot(root).render(
      <BootstrapError message={`Viewer data is invalid: ${String(error)}`} />,
    );
  }
}

start();
