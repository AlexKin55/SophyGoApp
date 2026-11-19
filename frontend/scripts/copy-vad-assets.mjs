// Копирует Silero VAD и ONNX Runtime Web в public/ для self-hosting.
// Запуск: npm run vad:assets
import { copyFile, mkdir } from 'node:fs/promises'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')

const files = [
  ['node_modules/@ricky0123/vad-web/dist/silero_vad_v5.onnx', 'public/vad/silero_vad_v5.onnx'],
  [
    'node_modules/@ricky0123/vad-web/dist/vad.worklet.bundle.min.js',
    'public/vad/vad.worklet.bundle.min.js',
  ],
  [
    'node_modules/onnxruntime-web/dist/ort-wasm-simd-threaded.mjs',
    'public/ort/ort-wasm-simd-threaded.mjs',
  ],
  [
    'node_modules/onnxruntime-web/dist/ort-wasm-simd-threaded.wasm',
    'public/ort/ort-wasm-simd-threaded.wasm',
  ],
]

for (const [source, target] of files) {
  const targetPath = resolve(root, target)
  await mkdir(dirname(targetPath), { recursive: true })
  await copyFile(resolve(root, source), targetPath)
  console.log(`${source} -> ${target}`)
}

console.log('Готово. По умолчанию приложение использует /vad/ и /ort/.')
