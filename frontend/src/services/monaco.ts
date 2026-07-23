import EditorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker'

let configured = false

export async function loadMonaco() {
  if (!configured) {
    const target = self as typeof self & {
      MonacoEnvironment?: { getWorker: () => Worker }
    }
    target.MonacoEnvironment = { getWorker: () => new EditorWorker() }
    configured = true
  }
  return import('monaco-editor/esm/vs/editor/editor.api')
}
