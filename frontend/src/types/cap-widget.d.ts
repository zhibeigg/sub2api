// Let vue-tsc accept the Cap (trycap.dev) <cap-widget> Web Component in
// SFC templates by registering it as a loosely-typed global component.
import type { DefineComponent } from 'vue'

declare module '@vue/runtime-core' {
  export interface GlobalComponents {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    'cap-widget': DefineComponent<Record<string, any>>
  }
}

export {}
