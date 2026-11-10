package battle

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const ResourceURI = "ui://totalbotwar/battlefield-v1.html"
const MIMEType = "text/html;profile=mcp-app"

//go:embed battlefield.html
var widgetHTML string

type object = map[string]any

func NewServer(version, dir string) *server.MCPServer {
	store := NewStore(dir)
	s := server.NewMCPServer("TotalBotWar", version, server.WithToolCapabilities(false), server.WithResourceCapabilities(false, false), server.WithInputSchemaValidation())
	s.AddResource(mcp.NewResource(ResourceURI, "TotalBotWar", mcp.WithMIMEType(MIMEType)), func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return []mcp.ResourceContents{mcp.TextResourceContents{URI: ResourceURI, MIMEType: MIMEType, Text: widgetHTML, Meta: object{"openai/ui": object{"availableDisplayModes": []string{"fullscreen"}, "preferredDisplayMode": "fullscreen"}, "ui": object{"prefersBorder": false, "csp": object{"connectDomains": []string{}, "resourceDomains": []string{}}}}}}, nil
	})
	add := func(name, title, description string, properties object, required []string, read, render bool, handler func(mcp.CallToolRequest) (any, error)) {
		raw, _ := json.Marshal(object{"type": "object", "properties": properties, "required": required, "additionalProperties": false})
		tool := mcp.NewTool(name, mcp.WithRawInputSchema(json.RawMessage(raw)), mcp.WithToolTitle(title), mcp.WithDescription(description), mcp.WithReadOnlyHintAnnotation(read), mcp.WithDestructiveHintAnnotation(false), mcp.WithOpenWorldHintAnnotation(false), mcp.WithIdempotentHintAnnotation(read))
		tool.InputSchema = mcp.ToolInputSchema{}
		meta := object{"ui": object{"visibility": []string{"model", "app"}}}
		if render {
			meta["ui"] = object{"resourceUri": ResourceURI, "visibility": []string{"model", "app"}}
			meta["openai/ui"] = object{"entrypoints": []object{{"type": "global"}, {"type": "thread"}}}
		}
		tool.Meta = mcp.NewMetaFromMap(meta)
		s.AddTool(tool, func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			value, err := handler(req)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			raw, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResultStructured(value, string(raw)), nil
		})
	}
	id := object{"type": "string", "pattern": "^[a-f0-9]{48}$", "description": "Private battle capability returned by create_battle. Never invent an ID."}
	rev := object{"type": "integer", "minimum": 1, "description": "Latest revision from get_battle; prevents stale chat/UI writes and duplicate turns."}
	add("open_battlefield", "TotalBotWar", "Open the fullscreen TotalBotWar battlefield from the sidebar or chat. With battle_id, resume that battle; without it, show the lobby. Create a battle using create_battle. Other tools update the mounted app without rendering another widget.", object{"battle_id": id}, []string{}, true, true, func(r mcp.CallToolRequest) (any, error) {
		id, _ := r.GetArguments()["battle_id"].(string)
		if id == "" {
			return object{"lobby": true}, nil
		}
		return store.Access(id, nil)
	})
	add("create_battle", "Start battle", "Create a new Ashen Vale skirmish versus a tactical bot. Returns battle_id and the player's fog-filtered state. Save that ID for subsequent commands. Then call open_battlefield with battle_id to display. Coordinates are zero-based (A1 = x:0,y:0); tower I6 = x:8,y:5. One order per unit per turn. Unordered units defend. Reach 6 tower points or eliminate the opposing army; at turn 24 higher score wins.", object{}, []string{}, false, false, func(mcp.CallToolRequest) (any, error) { return store.Create() })
	add("get_battle", "Read battlefield", "Read the latest player-visible state, revision, reports and queued orders. Hidden enemies and enemy orders are never included. Always refresh before planning commands; do not infer hidden enemy positions from earlier sightings.", object{"battle_id": id}, []string{"battle_id"}, true, false, func(r mcp.CallToolRequest) (any, error) {
		id, _ := r.GetArguments()["battle_id"].(string)
		return store.Access(id, nil)
	})
	order := object{"type": "object", "properties": object{"unit_id": object{"type": "string"}, "action": object{"type": "string", "enum": []string{"move", "attack", "defend", "scout", "cancel"}}, "x": object{"type": "integer", "minimum": 0, "maximum": Width - 1}, "y": object{"type": "integer", "minimum": 0, "maximum": Height - 1}, "target_id": object{"type": "string"}}, "required": []string{"unit_id", "action"}, "additionalProperties": false}
	add("issue_orders", "Queue unit orders", "Atomically queue or replace 1–5 friendly orders; does NOT advance the turn. move requires x,y and a land path within movement: infantry/spears/archers 2, cavalry 3, scouts 4. attack requires a visible target_id in range (archers 3, others 1), with line of sight. defend reduces damage and restores morale. scout extends vision during resolution and next planning phase. cancel removes a queued order. Ask the user's intent before ending a turn unless they already requested resolution. On stale revision refresh; never blindly retry writes.", object{"battle_id": id, "revision": rev, "orders": object{"type": "array", "items": order, "minItems": 1, "maxItems": 5}}, []string{"battle_id", "revision", "orders"}, false, false, func(r mcp.CallToolRequest) (any, error) {
		var in struct {
			ID       string  `json:"battle_id"`
			Revision int     `json:"revision"`
			Orders   []Order `json:"orders"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		raw, _ := json.Marshal(r.GetArguments()["orders"])
		var entries []map[string]any
		_ = json.Unmarshal(raw, &entries)
		for _, o := range entries {
			if o["action"] == "move" && (o["x"] == nil || o["y"] == nil) {
				return nil, fmt.Errorf("move requires both x and y")
			}
		}
		return store.Access(in.ID, func(b *Battle) error { return b.Issue(in.Revision, in.Orders) })
	})
	add("end_turn", "Resolve turn", "Commit queued orders and resolve both armies, then return the fog-filtered next planning phase or victory/defeat/draw. Unordered units defend. Movement precedes simultaneous attacks; targets can escape. Tower occupation scores 1 point each turn. Requires the latest revision. This advances the game; do not call just to inspect the battle.", object{"battle_id": id, "revision": rev}, []string{"battle_id", "revision"}, false, false, func(r mcp.CallToolRequest) (any, error) {
		var in struct {
			ID       string `json:"battle_id"`
			Revision int    `json:"revision"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return store.Access(in.ID, func(b *Battle) error { return b.End(in.Revision) })
	})
	return s
}
func decode(r mcp.CallToolRequest, out any) error {
	raw, err := json.Marshal(r.GetArguments())
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}
