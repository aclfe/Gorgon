## Gorgon v0.3.0

Go mutation testing tool.

## Mutations

Arithmetic
- `arithmetic_flip` - + ↔ -, * ↔ /

Logical
- `condition_negation` - == ↔ !=, < ↔ >=, etc.


Reference Returns
- `zero_value_return` - Replace literals with zero values
- `pointer_returns` - return &x → return nil
- `slice_returns` - return []T{} → return nil
- `map_returns` - return map[K]V{} → return nil
- `channel_returns` - return make(chan T) → return nil
- `interface_returns` - return "foo" → return nil (interface{} only)

## Engine

- Context-aware - passes type info to mutators
- Extensible - implement Operator or ContextualOperator interface
- Schemata-based for fast testing

## Usage

```
gorgon ./path/to/code
gorgon -operators=arithmetic_flip,condition_negation ./path
```
