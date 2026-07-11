export type PlaygroundCapability = 'chat' | 'image' | 'video'
export type PlaygroundMode = PlaygroundCapability | 'compare'

export interface PlaygroundModelOption {
  id?: string
  group_id: number
  group_name: string
  group_priority: number
  model: string
  platform: string
  capabilities: PlaygroundCapability[]
}

export function playgroundOptionKey(option: PlaygroundModelOption): string {
  return option.id || `${option.group_id}:${option.platform}:${option.model}`
}

export function samePlaygroundOption(
  left: PlaygroundModelOption | null | undefined,
  right: PlaygroundModelOption | null | undefined
): boolean {
  return !!left && !!right && playgroundOptionKey(left) === playgroundOptionKey(right)
}
