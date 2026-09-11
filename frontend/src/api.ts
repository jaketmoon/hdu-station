export type Conversation = { id: string; title: string; updatedAt: string };
export type Message = {
  id: string;
  conversationId: string;
  role: "user" | "assistant";
  content: string;
  state: string;
  createdAt: string;
};
export type Settings = {
  baseURL: string;
  model: string;
  hasAPIKey: boolean;
  qqStatus: string;
  dataRoot: string;
  zanao: {
    enabled: boolean;
    schoolAlias: string;
    hasToken: boolean;
    status: string;
  };
  xiaohongshu: {
    enabled: boolean;
    baseURL: string;
    hasAuthToken: boolean;
    status: string;
  };
};
export type SettingsInput = {
  baseURL: string;
  apiKey: string;
  zanao: {
    enabled: boolean;
    schoolAlias: string;
    token: string;
    clearToken: boolean;
  };
  xiaohongshu: {
    enabled: boolean;
    baseURL: string;
    authToken: string;
    clearAuthToken: boolean;
  };
};
export type TurnResult = {
  conversation: Conversation;
  message: Message;
  error?: string;
};
export type TurnEvent = {
  requestId: string;
  conversationId: string;
  kind: string;
  text: string;
  conversation?: Conversation;
  user?: Message;
  assistant?: Message;
};
type Bindings = {
  ListConversations(): Promise<Conversation[]>;
  GetMessages(id: string): Promise<Message[]>;
  Chat(id: string, question: string, requestId: string): Promise<TurnResult>;
  Cancel(requestId: string): Promise<void>;
  DeleteConversation(id: string): Promise<void>;
  GetSettings(): Promise<Settings>;
  SaveSettings(input: SettingsInput): Promise<Settings>;
  InstallQQ(): Promise<Settings>;
  OpenLink(url: string): Promise<void>;
  CopyText(text: string): Promise<void>;
};
declare global {
  interface Window {
    go?: { main: { App: Bindings } };
    runtime?: {
      EventsOn(name: string, handler: (event: TurnEvent) => void): () => void;
    };
  }
}
function bindings(): Bindings {
  if (!window.go?.main?.App)
    throw new Error("请打开 HDU Station 桌面应用以连接选课助手。");
  return window.go.main.App;
}
export const api = {
  list: () => bindings().ListConversations(),
  messages: (id: string) => bindings().GetMessages(id),
  chat: (id: string, text: string, requestId: string) =>
    bindings().Chat(id, text, requestId),
  cancel: (id: string) => bindings().Cancel(id),
  remove: (id: string) => bindings().DeleteConversation(id),
  settings: () => bindings().GetSettings(),
  save: (input: SettingsInput) => bindings().SaveSettings(input),
  install: () => bindings().InstallQQ(),
  open: (url: string) => bindings().OpenLink(url),
  copy: (text: string) => bindings().CopyText(text),
  subscribe: (handler: (event: TurnEvent) => void) =>
    window.runtime?.EventsOn("course:turn", handler) ?? (() => {}),
};
export function errorText(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}
