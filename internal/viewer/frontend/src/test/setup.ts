import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach, vi } from "vitest";

class IntersectionObserverMock implements IntersectionObserver {
  readonly root = null;
  readonly rootMargin = "0px";
  readonly scrollMargin = "0px";
  readonly thresholds = [0];

  constructor(
    _callback: IntersectionObserverCallback,
    _options?: IntersectionObserverInit,
  ) {}

  disconnect = vi.fn();
  observe = vi.fn();
  takeRecords = vi.fn(() => []);
  unobserve = vi.fn();
}

vi.stubGlobal("IntersectionObserver", IntersectionObserverMock);

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  document.body.replaceChildren();
  document.body.removeAttribute("data-view");
  history.replaceState(null, "", "/");
});
