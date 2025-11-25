package main

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

var CQRSAnalyzer = &analysis.Analyzer{
	Name: "cqrsnaming",
	Doc:  "ensures CQRS commands and events are used correctly: types ending with 'Command' must be used with CommandBus.Send, types ending with 'Event' must be used with EventBus.Publish",
	Run:  run,
}

func New(conf any) ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{CQRSAnalyzer}, nil
}

func run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			callExpr, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			// Check for CommandBus.Send or EventBus.Publish calls
			methodName := selExpr.Sel.Name
			if methodName != "Send" && methodName != "Publish" {
				return true
			}

			// Get the receiver type
			receiverType := getReceiverType(pass, selExpr.X)
			if receiverType == "" {
				return true
			}

			// Check if it's CommandBus or EventBus
			isCommandBus := strings.Contains(receiverType, "CommandBus")
			isEventBus := strings.Contains(receiverType, "EventBus")

			if !isCommandBus && !isEventBus {
				return true
			}

			// Get the argument type (command or event)
			if len(callExpr.Args) == 0 {
				return true
			}

			argType := getArgumentType(pass, callExpr.Args[0])
			if argType == "" {
				return true
			}

			// Validate naming convention
			isCommand := strings.HasSuffix(argType, "Command")
			isEvent := strings.HasSuffix(argType, "Event")

			// Report errors
			if isCommandBus && methodName == "Send" {
				if isEvent {
					pass.Report(analysis.Diagnostic{
						Pos:      callExpr.Pos(),
						End:      callExpr.End(),
						Category: "cqrs",
						Message:  "Event type '" + argType + "' should not be used with CommandBus.Send. Use EventBus.Publish instead.",
					})
				} else if !isCommand {
					// Warn if type doesn't follow naming convention
					pass.Report(analysis.Diagnostic{
						Pos:      callExpr.Pos(),
						End:      callExpr.End(),
						Category: "cqrs",
						Message:  "Type '" + argType + "' used with CommandBus.Send should end with 'Command' suffix.",
					})
				}
			}

			if isEventBus && methodName == "Publish" {
				if isCommand {
					pass.Report(analysis.Diagnostic{
						Pos:      callExpr.Pos(),
						End:      callExpr.End(),
						Category: "cqrs",
						Message:  "Command type '" + argType + "' should not be used with EventBus.Publish. Use CommandBus.Send instead.",
					})
				} else if !isEvent {
					// Warn if type doesn't follow naming convention
					pass.Report(analysis.Diagnostic{
						Pos:      callExpr.Pos(),
						End:      callExpr.End(),
						Category: "cqrs",
						Message:  "Type '" + argType + "' used with EventBus.Publish should end with 'Event' suffix.",
					})
				}
			}

			return true
		})
	}

	return nil, nil
}

func getReceiverType(pass *analysis.Pass, expr ast.Expr) string {
	tv, ok := pass.TypesInfo.Types[expr]
	if !ok {
		return ""
	}

	if tv.Type == nil {
		return ""
	}

	return tv.Type.String()
}

func getArgumentType(pass *analysis.Pass, expr ast.Expr) string {
	// Handle composite literals: &TypeName{...}
	if unary, ok := expr.(*ast.UnaryExpr); ok && unary.Op.String() == "&" {
		if composite, ok := unary.X.(*ast.CompositeLit); ok {
			if ident, ok := composite.Type.(*ast.Ident); ok {
				return ident.Name
			}
			if sel, ok := composite.Type.(*ast.SelectorExpr); ok {
				return sel.Sel.Name
			}
		}
		// Try to get type from the unary expression
		expr = unary.X
	}

	// Handle composite literals: TypeName{...}
	if composite, ok := expr.(*ast.CompositeLit); ok {
		if ident, ok := composite.Type.(*ast.Ident); ok {
			return ident.Name
		}
		if sel, ok := composite.Type.(*ast.SelectorExpr); ok {
			return sel.Sel.Name
		}
	}

	// Try to get type from types.Info
	tv, ok := pass.TypesInfo.Types[expr]
	if ok && tv.Type != nil {
		typeStr := tv.Type.String()
		// Handle pointer types (*TypeName)
		typeStr = strings.TrimPrefix(typeStr, "*")
		// Handle package-qualified types (package.TypeName)
		parts := strings.Split(typeStr, ".")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
		return typeStr
	}

	// Try to get type from ident
	if ident, ok := expr.(*ast.Ident); ok {
		if obj := pass.TypesInfo.ObjectOf(ident); obj != nil {
			if objType, ok := obj.Type().(*types.Named); ok {
				return objType.Obj().Name()
			}
			// Handle pointer types
			if ptr, ok := obj.Type().(*types.Pointer); ok {
				if named, ok := ptr.Elem().(*types.Named); ok {
					return named.Obj().Name()
				}
			}
		}
	}

	return ""
}
