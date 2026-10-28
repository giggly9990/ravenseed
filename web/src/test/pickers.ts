import { fireEvent, screen, waitFor } from "@testing-library/react";

export async function openPicker(name: string | HTMLElement) {
  const button =
    typeof name === "string" ? screen.getByRole("button", { name }) : name;
  await waitFor(() => expectEnabled(button));
  fireEvent.keyDown(button, { key: "ArrowDown" });
  return screen.findByRole("menu");
}

function expectEnabled(button: HTMLElement) {
  // :disabled also covers a disabled <fieldset> ancestor.
  if (button.matches(":disabled")) throw new Error("Picker is disabled");
}

export async function pick(name: string | HTMLElement, option: string) {
  await openPicker(name);
  fireEvent.click(await screen.findByRole("menuitemradio", { name: option }));
}

export function closePicker() {
  fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });
}
