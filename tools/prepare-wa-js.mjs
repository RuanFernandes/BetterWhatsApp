import { access, copyFile, mkdir } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const projectRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const sourceDirectory = resolve(projectRoot, "node_modules/@wppconnect/wa-js/dist");
const sourceBundle = resolve(sourceDirectory, "wppconnect-wa.js");
const sourceLicense = resolve(sourceDirectory, "wppconnect-wa.js.LICENSE.txt");
const targetDirectory = resolve(projectRoot, "assets/injector");
const targetBundle = resolve(targetDirectory, "wa-js.js");
const targetLicense = resolve(targetDirectory, "wa-js.LICENSE.txt");

try {
  await access(sourceBundle);
  await access(sourceLicense);
} catch {
  throw new Error(
    "WA-JS is not installed. Run `npm install` at the project root first.",
  );
}

await mkdir(targetDirectory, { recursive: true });
await copyFile(sourceBundle, targetBundle);
await copyFile(sourceLicense, targetLicense);
console.log("Prepared local WA-JS bundle and license in assets/injector/");
