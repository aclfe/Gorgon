## Gorgon v0.5.0

Go mutation testing tool.

## Mutations

Arithmetic
- `arithmetic_flip` - + ↔ -, * ↔ /

Logical
- `condition_negation` - == ↔ !=, < ↔ >=, <= ↔ >, > ↔ <=
- `negate_condition` - if (x) → if (!x)
- `logical_operator` - && ↔ ||

Boundary
- `boundary_value` - < ↔ <=, > ↔ >=

Assignment
- `assignment_operator` - = → +=, += ↔ -=, *= ↔ /=

Function Body
- `empty_body` - Replace void function body with {}

Binary Operators
- `binary_math` - % ↔ *, & ↔ |, << ↔ >>
- `inc_dec_flip` - ++ ↔ --
- `sign_toggle` - Unary -x ↔ +x

Literal
- `constant_replacement` - Replace literals with different values
- `variable_replacement` - Replace variable with another of same type
- `zero_value_return_numeric` - Replace numeric literals with 0
- `zero_value_return_string` - Replace string literals with ""
- `zero_value_return_bool` - Replace bool literals with false
- `zero_value_return_error` - Replace fmt.Errorf() with nil

Early Return
- `early_return_removal` - Remove early return statements inside if blocks

Reference Returns
- `pointer_returns` - return &x → return nil
- `slice_returns` - return []T{} → return nil
- `map_returns` - return map[K]V{} → return nil
- `channel_returns` - return make(chan T) → return nil
- `interface_returns` - return "foo" → return nil (interface{} only)

Switch
- `switch_remove_default` - Remove default case from switch
- `swap_case_bodies` - Swap case bodies within same switch

Conditional Expression
- `if_condition_true` - if (a > b) → if (true)
- `if_condition_false` - if (a > b) → if (false)
- `for_condition_true` - for i < 10 {} → for true {}
- `for_condition_false` - for i < 10 {} → for false {}

Loop
- `loop_body_removal` - Remove loop body, leaving empty loop
- `loop_break_first` - Add break after first iteration
- `loop_break_removal` - Remove break statements inside loops

Statement
- `defer_removal` - Remove defer statements

## Engine

- Context-aware - passes type info to mutators
- Extensible - implement Operator or ContextualOperator interface
- Schemata-based for fast testing

## Usage

```
gorgon ./path/to/code
gorgon -operators=arithmetic,logical ./path
gorgon -operators=binary ./path
```
