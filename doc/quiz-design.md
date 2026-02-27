# Designing Symmetric Configurations for Gorums: The Quiz Game Architecture

This document describes the architectural pattern for using Gorums symmetric configurations (`gorums.WithClientConfig`) in a server-client broadcast scenario.

## The Architecture

In symmetric configurations, the standard roles of "Server" and "Client" are blurred over a single underlying `NodeStream` connection. The "Server" acts as a listener who assigns dynamically connected users network IDs, and the "Client" initiates the connection but also configures itself to receive inbound RPCs.

A perfect use-case for this is a live game, where the game server needs to synchronously broadcast data and collect responses from players after they initially connect.

We can define two distinct protobuf services for this: `Quiz` and `Bootstrap`.

```proto
// Quiz defines the main quiz interactions between quizmaster and players.
service Quiz {
  // Question pushes a question to players.
  // It is a Quorum Call that returns a stream of answers.
  // Players can update their answer multiple times.
  rpc Question(QuestionRequest) returns (stream AnswerResponse) {
    option (gorums.quorumcall) = true;
  }

  // ShowResult pushes the correct answer and results for the round.
  // Multicast to all players.
  rpc ShowResult(ResultRequest) returns (ResultResponse) {
    option (gorums.multicast) = true;
  }
}

// Bootstrap for scenarios where UDP broadcast is not available.
// The player connects to the quizmaster to register its presence.
service Bootstrap {
  // Register a player with the quizmaster
  rpc Register(RegisterRequest) returns (RegisterResponse) {
      option (gorums.grpccall) = true;
  }
}
```

## How It Works in Practice

### 1. Client Initialization

The Player (Client) creates a `gorums.System` (or `Manager`) and registers its local implementation of the `Quiz` service using `RegisterQuizServer(srv, playerImpl)`.

To allow the server to ping back, the player **must** enable symmetric routing on their outbound dial:

```go
// The player creates the connection with symmetric routing enabled
qCfg, err := sys.NewOutboundConfig(
    gorums.WithNodeList([]string{"quizmaster.games.local"}),
)
```

### 2. Server Authentication and Mapping

The Quizmaster (Server) accepts the `NodeStream` connections. The `InboundManager` assigns each connecting player a dynamic NodeID (e.g., `1048576`) and adds them to `ServerCtx.ClientConfig()`.

The player calls `Bootstrap.Register(...)`. Inside the Quizmaster's handler for this call, `ctx.Node().ID()` will be that dynamically assigned ID (`1048576`), and the request payload will contain application data (e.g., `playerName: "Alice"`).

The Quizmaster can now map the application state `"Alice" -> NodeID 1048576`.

### 3. Symmetrical Broadcasts

When the Quizmaster wants to push a `Question` to all players, it simply fetches the auto-updated client configuration and invokes the `gorums.QuorumCall`.

```go
// Inside Quizmaster backend event loop
cfg := server.ClientConfig() // Auto-maintained list of all connected players

responses := gorums.QuorumCall(
    cfg.Context(context.Background()),
    &QuestionRequest{...},
    "Quiz.Question",
)
```

Gorums effortlessly multiplexes the outgoing requests down the inbound channels back to the players, where their local `gorums.Server` handles it and sends the answers back up.

## Gotchas and Requirements

1. **Bootstrap Must Route Through Gorums:**
   The `Bootstrap.Register` RPC must be tagged with a Gorums call type (`(gorums.grpccall) = true`). If left as a standard single-use gRPC unary call, it will bypass `NodeStream` and open a standalone HTTP/2 stream, depriving the Quizmaster handler of the player's symmetric `NodeID`.

2. **Targeting Subsets of Players:**
   `ServerCtx.ClientConfig()` returns a configuration containing *all* dynamically connected clients. To target a specific subset of players (e.g., team-based questions), the Quizmaster must manually build a sub-configuration using `manager.NewConfiguration(opts...)`, passing only the specific NodeIDs it wants to target.
