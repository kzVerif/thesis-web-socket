# Frontend screen streaming

Connect the frontend to:

```text
ws://localhost:8081/ws/frontend
```

Start all online agents assigned to a room:

```json
{"type":"screen","action":"start","room_id":"ROOM_UUID"}
```

Stop when leaving the screen page:

```json
{"type":"screen","action":"stop","room_id":"ROOM_UUID"}
```

The server reference-counts viewers. It broadcasts `screen/start` to every online
agent in the room when the first viewer subscribes and broadcasts `screen/stop` when
the last viewer unsubscribes or disconnects. An agent that reconnects while its room
is being viewed receives `screen/start` automatically.

## Receiving frames

For each frame, the frontend receives two consecutive WebSocket messages:

1. A JSON header identifying the source agent.
2. The raw JPEG as a binary message.

```json
{"type":"screen","agent_id":"AGENT_UUID","room_id":"ROOM_UUID"}
```

The pair cannot be interleaved with another agent's frame on the same frontend
connection. Example:

```javascript
const socket = new WebSocket("ws://localhost:8081/ws/frontend");
socket.binaryType = "arraybuffer";

const imageUrls = new Map();
let pendingFrame;

socket.onopen = () => socket.send(JSON.stringify({
  type: "screen",
  action: "start",
  room_id: roomId,
}));

socket.onmessage = ({ data }) => {
  if (typeof data === "string") {
    const message = JSON.parse(data);
    if (message.type === "screen" && message.agent_id) pendingFrame = message;
    return;
  }
  if (!pendingFrame) return;

  const agentId = pendingFrame.agent_id;
  pendingFrame = undefined;
  const nextUrl = URL.createObjectURL(new Blob([data], { type: "image/jpeg" }));
  const image = document.querySelector(`[data-agent-id="${agentId}"]`);
  if (image) image.src = nextUrl;
  const previousUrl = imageUrls.get(agentId);
  if (previousUrl) URL.revokeObjectURL(previousUrl);
  imageUrls.set(agentId, nextUrl);
};
```
