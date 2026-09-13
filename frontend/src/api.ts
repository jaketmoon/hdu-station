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
  campus: CampusConnection;
  qqStatus: string;
  qqEnabled: boolean;
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
  xiaohongshu?: {
    enabled: boolean;
    baseURL: string;
    authToken: string;
    clearAuthToken: boolean;
  };
};
export type LoginSource = "qq" | "xiaohongshu";
export type CampusConnection = {
  scheduleAccess?: boolean;
  hasCredential: boolean;
  status: string;
  message?: string;
};
export type CampusLogin = {
  id: string;
  status: string;
  userCode?: string;
  expiresAt: number;
  message?: string;
};
export type SourceConnection = { enabled: boolean; status: string };
export type SourceLogin = {
  id: string;
  status: string;
  image?: string;
  expiresAt?: number;
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
  CheckCampus(): Promise<CampusConnection>;
  BeginCampusLogin(): Promise<CampusLogin>;
  PollCampusLogin(id: string): Promise<CampusLogin>;
  CancelCampusLogin(id: string): Promise<CampusLogin>;
  OpenCampusLogin(id: string): Promise<void>;
  LogoutCampus(): Promise<CampusConnection>;
  InstallQQ(): Promise<Settings>;
  CheckSource(source: LoginSource): Promise<SourceConnection>;
  SetSourceEnabled(
    source: LoginSource,
    enabled: boolean,
  ): Promise<SourceConnection>;
  BeginSourceLogin(source: LoginSource): Promise<SourceLogin>;
  PollSourceLogin(id: string): Promise<SourceLogin>;
  CancelSourceLogin(id: string): Promise<void>;
  ClearSourceCredentials(source: LoginSource): Promise<SourceConnection>;
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
  checkCampus: () => bindings().CheckCampus(),
  beginCampusLogin: () => bindings().BeginCampusLogin(),
  pollCampusLogin: (id: string) => bindings().PollCampusLogin(id),
  cancelCampusLogin: (id: string) => bindings().CancelCampusLogin(id),
  openCampusLogin: (id: string) => bindings().OpenCampusLogin(id),
  logoutCampus: () => bindings().LogoutCampus(),
  install: () => bindings().InstallQQ(),
  checkSource: (source: LoginSource) => bindings().CheckSource(source),
  enableSource: (source: LoginSource, enabled: boolean) =>
    bindings().SetSourceEnabled(source, enabled),
  beginLogin: (source: LoginSource) => bindings().BeginSourceLogin(source),
  pollLogin: (id: string) => bindings().PollSourceLogin(id),
  cancelLogin: (id: string) => bindings().CancelSourceLogin(id),
  clearSource: (source: LoginSource) =>
    bindings().ClearSourceCredentials(source),
  open: (url: string) => bindings().OpenLink(url),
  copy: (text: string) => bindings().CopyText(text),
  subscribe: (handler: (event: TurnEvent) => void) =>
    window.runtime?.EventsOn("course:turn", handler) ?? (() => {}),
};
export function errorText(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}
