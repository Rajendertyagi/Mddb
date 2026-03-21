// Outgoing messages (client → server)
export interface JoinMessage {
  type: 'join';
  name: string;
  scenario: string;
}

export interface ChatMessage {
  type: 'message';
  content: string;
}

export interface ResumeMessage {
  type: 'resume';
  session_id: string;
}

export interface EndMessage {
  type: 'end';
}

export interface PingMessage {
  type: 'ping';
}

export interface FeedbackMessage {
  type: 'feedback';
  rating: 'up' | 'down';
  question: string;
  answer: string;
}

export type WsOutgoing = JoinMessage | ChatMessage | ResumeMessage | EndMessage | FeedbackMessage | PingMessage;

// Incoming messages (server → client)
export interface SessionMessage {
  type: 'session';
  id: string;
  scenario: string;
}

export interface QueuedMessage {
  type: 'queued';
  position: number;
}

export interface ChunkMessage {
  type: 'chunk';
  content: string;
}

export interface DoneMessage {
  type: 'done';
}

export interface ErrorMessage {
  type: 'error';
  message: string;
}

export interface PongMessage {
  type: 'pong';
}

export interface EndedMessage {
  type: 'ended';
}

export type WsIncoming =
  | SessionMessage
  | QueuedMessage
  | ChunkMessage
  | DoneMessage
  | ErrorMessage
  | PongMessage
  | EndedMessage;
