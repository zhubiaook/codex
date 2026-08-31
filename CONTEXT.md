# Codex SDK

The Codex SDK context defines the language shared by language-specific libraries that embed the Codex agent in applications and workflows.

## Language

**Thread**:
A persisted conversation with the Codex agent. A thread contains one or more consecutive turns and can be resumed by its identifier.
_Avoid_: Session, conversation object

**Turn**:
One request-and-response cycle within a thread, beginning with user input and ending in completion or failure.
_Avoid_: Request, invocation

**Thread Item**:
A structured unit of agent activity produced during a turn, such as an agent message, reasoning summary, command execution, or file change.
_Avoid_: Message, event

**Thread Event**:
A lifecycle notification emitted while a turn runs. An event may describe a thread or turn transition, or carry a thread item.
_Avoid_: Callback, message
