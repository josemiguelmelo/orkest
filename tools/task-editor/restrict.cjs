// Spike-only replacement for the pinned node-pty entry point in the RO image.
// Immutable with the container root; does not depend on inherited Node options.
// Requalify on every code-server upgrade. This is not a public upstream API.
function deny() {
  throw new Error("Task editor is read-only: terminal and tasks are disabled");
}
exports.spawn = deny;
exports.fork = deny;
exports.createTerminal = deny;
exports.open = deny;
exports.native = null;
