BetterWhatsApp.registerPlugin("quick-reactions", ({ addStyle, observe, log }) => {
  addStyle(
    "plugin-quick-reactions",
    [
      "[data-better-whatsapp='quick-reactions'] {",
      "  align-items: center;",
      "  background: transparent;",
      "  border: 0;",
      "  color: var(--bw-accent, #25d366);",
      "  cursor: pointer;",
      "  display: inline-flex;",
      "  font-size: 18px;",
      "  height: 36px;",
      "  justify-content: center;",
      "  margin-inline: 4px;",
      "  width: 36px;",
      "}",
      "[data-better-whatsapp='quick-reactions']:hover {",
      "  background: rgba(37, 211, 102, 0.12);",
      "  border-radius: 50%;",
      "}",
    ].join("\n"),
  );

  observe("[contenteditable='true']", (composer) => {
    const parent = composer.parentElement;
    if (!parent || parent.querySelector("[data-better-whatsapp='quick-reactions']")) {
      return;
    }

    const button = document.createElement("button");
    button.type = "button";
    button.dataset.betterWhatsapp = "quick-reactions";
    button.title = "Inserir uma reação rápida";
    button.textContent = "✨";
    button.addEventListener("click", () => {
      composer.focus();
      document.execCommand("insertText", false, "✨ ");
    });
    parent.appendChild(button);
  });

  log("quick reactions mounted");
});
