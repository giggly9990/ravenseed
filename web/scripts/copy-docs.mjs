import { copyFile, mkdir } from "node:fs/promises";
await mkdir("dist/docs-assets", { recursive: true });
for (const file of ["swagger-ui-bundle.js", "swagger-ui.css"]) {
  await copyFile(
    `node_modules/swagger-ui-dist/${file}`,
    `dist/docs-assets/${file}`,
  );
}
