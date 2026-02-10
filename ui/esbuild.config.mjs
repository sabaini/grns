import { build, context } from 'esbuild';
import fs from 'node:fs/promises';
import path from 'node:path';

const watch = process.argv.includes('--watch');
const rootDir = process.cwd();
const outdir = path.resolve(rootDir, '../internal/server/uiassets/dist');

function normalizePath(p) {
  return p.replaceAll('\\\\', '/');
}

function pickEntryOutput(metafile, entrySuffix, ext) {
  const outputs = Object.entries(metafile.outputs);
  for (const [outfile, meta] of outputs) {
    const entryPoint = meta.entryPoint ? normalizePath(meta.entryPoint) : '';
    if (entryPoint.endsWith(entrySuffix) && outfile.endsWith(ext)) {
      return path.basename(outfile);
    }
  }
  return '';
}

async function writeIndex(metafile) {
  const jsFile = pickEntryOutput(metafile, 'src/index.jsx', '.js');
  const cssFile = pickEntryOutput(metafile, 'src/app.css', '.css');
  if (!jsFile || !cssFile) {
    throw new Error('build outputs missing JS or CSS entry');
  }

  const html = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>grns</title>
    <link rel="stylesheet" href="/ui/${cssFile}" />
  </head>
  <body>
    <div id="app"></div>
    <script type="module" src="/ui/${jsFile}"></script>
  </body>
</html>
`;

  await fs.writeFile(path.join(outdir, 'index.html'), html);
}

const writeIndexPlugin = {
  name: 'write-index',
  setup(buildCtx) {
    buildCtx.onEnd(async (result) => {
      if (result.errors.length > 0 || !result.metafile) {
        return;
      }
      await writeIndex(result.metafile);
    });
  },
};

const config = {
  entryPoints: {
    index: 'src/index.jsx',
    app: 'src/app.css',
  },
  bundle: true,
  format: 'esm',
  splitting: false,
  target: ['es2020'],
  outdir,
  jsx: 'automatic',
  jsxImportSource: 'preact',
  metafile: true,
  minify: !watch,
  sourcemap: watch ? 'inline' : false,
  entryNames: '[name].[hash]',
  assetNames: '[name].[hash]',
  loader: {
    '.svg': 'file',
  },
  plugins: [writeIndexPlugin],
};

await fs.rm(outdir, { recursive: true, force: true });
await fs.mkdir(outdir, { recursive: true });

if (watch) {
  const ctx = await context(config);
  await ctx.watch();
  // Keep process alive in watch mode.
  await new Promise(() => {});
} else {
  await build(config);
}
