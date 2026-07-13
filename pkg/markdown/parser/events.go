package parser

import "github.com/cjccjj/mdflow/pkg/markdown/event"

// EventType is retained as an alias for source compatibility. New consumers
// should import package event directly.
type EventType = event.Type

// Event is retained as an alias for source compatibility. New consumers
// should import package event directly.
type Event = event.Event

const (
	TextEvent               = event.TextEvent
	HeaderStartEvent        = event.HeaderStartEvent
	HeaderEndEvent          = event.HeaderEndEvent
	BoldStartEvent          = event.BoldStartEvent
	BoldEndEvent            = event.BoldEndEvent
	ItalicStartEvent        = event.ItalicStartEvent
	ItalicEndEvent          = event.ItalicEndEvent
	StrikethroughStartEvent = event.StrikethroughStartEvent
	StrikethroughEndEvent   = event.StrikethroughEndEvent
	InlineCodeStartEvent    = event.InlineCodeStartEvent
	InlineCodeEndEvent      = event.InlineCodeEndEvent
	CodeBlockStartEvent     = event.CodeBlockStartEvent
	CodeBlockEndEvent       = event.CodeBlockEndEvent
	CodeBlockLangEvent      = event.CodeBlockLangEvent
	HorizontalRuleEvent     = event.HorizontalRuleEvent
	BulletItemEvent         = event.BulletItemEvent
	NewlineEvent            = event.NewlineEvent
	TableStartEvent         = event.TableStartEvent
	TableRowEvent           = event.TableRowEvent
	TableEndEvent           = event.TableEndEvent
	BlockquoteStartEvent    = event.BlockquoteStartEvent
	BlockquoteEndEvent      = event.BlockquoteEndEvent
	LinkEvent               = event.LinkEvent
	ImageEvent              = event.ImageEvent
	HTMLBlockStartEvent     = event.HTMLBlockStartEvent
	HTMLBlockEndEvent       = event.HTMLBlockEndEvent
	LinkRefDefEvent         = event.LinkRefDefEvent
	LinkRefEvent            = event.LinkRefEvent
	ImageRefEvent           = event.ImageRefEvent
	AutolinkURLEvent        = event.AutolinkURLEvent
	AutolinkEmailEvent      = event.AutolinkEmailEvent
)
