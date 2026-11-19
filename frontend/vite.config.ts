import { createReadStream, existsSync, statSync } from 'node:fs'
import { extname, join } from 'node:path'
import react from '@vitejs/plugin-react'
import { defineConfig, type Plugin } from 'vite'

/**
 * Vite в dev-режиме пытается трансформировать ESM-загрузчик ONNX Runtime
 * (ort-wasm-simd-threaded.mjs) и запрещает импорт файлов из public/.
 * Этот middleware отдаёт /vad/ и /ort/ напрямую, без import analysis.
 * В production файлы копируются из public/ и отдаются статически.
 */
function staticVadAssets(): Plugin {
  return {
    name: 'sophygo-static-vad-assets',
    configureServer(server) {
      server.middlewares.use((request, response, next) => {
        const pathname = (request.url ?? '').split('?')[0]
        if (!pathname.startsWith('/vad/') && !pathname.startsWith('/ort/')) {
          next()
          return
        }

        const filePath = join(server.config.publicDir, pathname)
        if (!filePath.startsWith(server.config.publicDir) || !existsSync(filePath) || !statSync(filePath).isFile()) {
          next()
          return
        }

        const extension = extname(filePath)
        const contentType =
          extension === '.wasm'
            ? 'application/wasm'
            : extension === '.mjs' || extension === '.js'
              ? 'text/javascript'
              : 'application/octet-stream'

        response.setHeader('Content-Type', contentType)
        response.setHeader('Cache-Control', 'no-cache')
        createReadStream(filePath).pipe(response)
      })
    },
  }
}

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), staticVadAssets()],
  server: {
    proxy: {
      // В dev-режиме запросы к backend уходят на локальный Go-сервис.
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        ws: true,
      },
    },
  },
})
