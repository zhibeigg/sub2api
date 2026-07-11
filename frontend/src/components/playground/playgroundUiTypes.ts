export type PlaygroundReasoningEffort = '' | 'none' | 'minimal' | 'low' | 'medium' | 'high' | 'xhigh'

export interface PlaygroundToolSettings {
  webSearch: boolean
  codeExecution: boolean
  webFetch: boolean
}

export interface PlaygroundParameterValues extends PlaygroundToolSettings {
  systemPrompt: string
  temperature: number
  topP: number
  maxTokens: number
  reasoningEffort: PlaygroundReasoningEffort
}

