import { build } from 'esbuild';
import { readFile, writeFile } from 'node:fs/promises';
const result = await build({
  entryPoints: [new URL('../web/battlefield.ts', import.meta.url).pathname],
  bundle: true, format: 'esm', platform: 'browser', target: 'es2022',
  minify: true, write: false, legalComments: 'inline',
});
const script = result.outputFiles[0].text.replace(/<\/script/gi, '<\\/script');
const shell = await readFile(new URL('../web/shell.html', import.meta.url), 'utf8');
await writeFile(new URL('../internal/battle/battlefield.html', import.meta.url), shell.replace('<!-- WIDGET_SCRIPT -->', () => `<script type="module">${script}</script>`));
console.log('Built internal/battle/battlefield.html');
