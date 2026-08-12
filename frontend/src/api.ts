export type CampusAuthStatus = {
  state: 'not_configured' | 'pat_configured_unverified' | 'pat_verified' | 'pat_rejected' | 'scope_missing' | 'unavailable'
  configured: boolean
  method: 'none' | 'pat'
  deviceAuthorization: 'unavailable'
  serverClientRegistrationRequired: boolean
  notice: string
}

export type BootstrapState = {
  name: string
  version: string
  platform: string
  dataRoot: string
  startupError?: string
  configuration: {
    campusConfigured: boolean
    campusAuth: CampusAuthStatus
    defaultProvider: string
    configuredProviders: string[]
  }
}

export type Conversation = {
  id: string
  title: string
  createdAt: string
  updatedAt: string
}

export type PersistedMessage = {
  id: string
  conversationId: string
  role: 'user' | 'assistant'
  content: string
  createdAt: string
}

export type SandboxStatus = {
  platform: string
  supported: boolean
  ready: boolean
  reason: string
}

export type TencentChannelStatus = {
  cliInstalled: boolean
  channelIndexConfigured: boolean
  searchAvailable: boolean
  notice: string
}

export type ToolDefinition = {
  name: string
  description: string
  parameters: unknown
}

export type ToolResult = {
  text: string
}

export type ToolAudit = {
  id: string
  conversationId: string
  toolName: string
  status: 'started' | 'completed' | 'failed'
  detail?: string
  createdAt: string
}

export type ProviderSettings = {
  name: string
  type: string
  protocol: string
  baseURL: string
  model: string
  configured: boolean
}

export type SettingsState = {
  campusConfigured: boolean
  campusAuth: CampusAuthStatus
  tencent: TencentChannelStatus
  webSearch: string
  providers: ProviderSettings[]
}

export type StationBindings = {
  Bootstrap: () => Promise<BootstrapState>
  Conversations: () => Promise<Conversation[]>
  CreateConversation: (title: string) => Promise<Conversation>
  Messages: (conversationID: string) => Promise<PersistedMessage[]>
  ToolAudits: (conversationID: string) => Promise<ToolAudit[]>
  AppendMessage: (
    conversationID: string,
    role: 'user' | 'assistant',
    content: string,
  ) => Promise<PersistedMessage>
  Chat: (conversationID: string, prompt: string) => Promise<PersistedMessage>
  ChatStream: (conversationID: string, prompt: string) => Promise<PersistedMessage>
  SandboxStatus: () => Promise<SandboxStatus>
  InstallSandbox: () => Promise<void>
  StartSandbox: () => Promise<void>
  StopSandbox: () => Promise<void>
  PurgeSandbox: () => Promise<void>
  ClearAllData: () => Promise<void>
  ClearCampusCredential: () => Promise<void>
  ImportHduhelpCLICredential: () => Promise<void>
  InstallTencentCLI: () => Promise<void>
  TencentChannelStatus: () => Promise<TencentChannelStatus>
  ReadOnlyTools: () => Promise<ToolDefinition[]>
  Tools?: () => Promise<ToolDefinition[]>
  CallReadOnlyTool: (name: string, argumentsJSON: string) => Promise<ToolResult>
  Settings: () => Promise<SettingsState>
  SaveModelProvider: (
    name: string,
    apiKey: string,
    model: string,
    baseURL: string,
    protocol: string,
  ) => Promise<void>
  SaveCampusKey: (key: string) => Promise<void>
}

type WailsGlobal = {
  go?: {
    main?: {
      App?: Partial<StationBindings>
    }
  }
}

export type ChatTokenEvent = {
  conversationId: string
  text: string
}

type WailsEvents = {
  EventsOn: (eventName: string, callback: (payload: ChatTokenEvent) => void) => (() => void) | void
  EventsOff?: (eventName: string) => void
}

type WailsEventsGlobal = {
  runtime?: WailsEvents
}

export function getStationBindings(): StationBindings | null {
  const candidate = (globalThis as WailsGlobal).go?.main?.App
  if (!candidate || typeof candidate.Bootstrap !== 'function') {
    return null
  }
  return candidate as StationBindings
}

export function getStationEvents(): WailsEvents | null {
  const candidate = (globalThis as WailsEventsGlobal).runtime
  if (!candidate || typeof candidate.EventsOn !== 'function') {
    return null
  }
  return candidate
}
