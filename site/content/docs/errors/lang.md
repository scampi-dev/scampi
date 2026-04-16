---
title: "lang.*"
weight: 3
---

Lang errors are caught by the type checker or evaluator. The file parses fine,
but the program has a semantic problem — a type mismatch, a missing field, an
undefined name. These run before any step touches a target, so they're cheap to
hit and fast to fix.

## lang.AmbiguousUFCS

A UFCS (uniform function call syntax) call matched multiple candidate functions
across different imported modules. The checker can't decide which one you meant.

{{< err >}}
module main
import "mod_a"
import "mod_b"
// both mod_a and mod_b export transform(x: int) int
let y = (5).transform() // ← ambiguous: mod_a.transform or mod_b.transform?
{{< /err >}}

Use the fully qualified form (`mod_a.transform(5)`) to disambiguate.

## lang.ArgTypeMismatch

A function argument doesn't match the expected parameter type.

{{< err >}}
module main
import "std"
import "std/local"
let host = local.target { name = "h" }
std.deploy("web", "not-a-list") { // ← targets expects list[Target]
}
{{< /err >}}

{{< fix >}}
module main
import "std"
import "std/local"
let host = local.target { name = "h" }
std.deploy("web", [host]) {
}
{{< /fix >}}

## lang.ArithNotInt

An arithmetic operator (`+`, `-`, `*`, `/`, `%`) was used on a non-integer value.

{{< err >}}
module main
let x = "hello" + 1 // ← can't add string and int
{{< /err >}}

String concatenation uses `+` between two strings. Mixing types requires
explicit conversion.

## lang.AttrError

An attribute failed its validation check. This is the generic fallback for
attribute errors that don't have a more specific code.

## lang.AttrFieldCarriesAttr

A field in a type declaration carries an attribute that's only valid on
parameters, or vice versa. Attributes have specific targets — not all
attributes apply everywhere.

## lang.AttrTooManyDots

An attribute name has too many dotted segments. Attribute names are at most
two segments: `@std.nonempty` (module + name).

{{< err >}}
module main
type Foo {
  @std.some.deeply.nested
  x: string
}
{{< /err >}}

## lang.BoolOpNotBool

A boolean operator (`&&`, `||`) was used on a non-boolean value.

{{< err >}}
module main
func f(x: int, y: int) bool {
  return x && y // ← && requires bool operands
}
{{< /err >}}

{{< fix >}}
module main
func f(x: int, y: int) bool {
  return x > 0 && y > 0
}
{{< /fix >}}

## lang.CallError

A function call failed at eval time. This covers UFCS resolution failures
and other runtime call errors not caught by the type checker.

## lang.CannotAccess

A field or member was accessed on a value that doesn't support it.

{{< err >}}
module main
let x = 42
let y = x.name // ← int has no fields
{{< /err >}}

This triggers on any `.name` access where the left side isn't a struct, map,
or module.

## lang.CannotAdd

The `+` operator was used on types that don't support addition. Only `int + int`
and `string + string` are valid.

{{< err >}}
module main
let x = [1, 2] + [3, 4] // ← can't add lists with +
{{< /err >}}

## lang.CannotCall

Something that isn't a function was used in a call expression.

{{< err >}}
module main
let x = 42
let y = x(1) // ← x is an int, not a function
{{< /err >}}

## lang.CannotEvaluate

The evaluator hit an expression it can't reduce. This usually means the
type checker missed something — file a bug if you see this.

## lang.CannotFillBlock

A trailing-block `{ ... }` was attached to a value that isn't a `block[T]`.

{{< err >}}
module main
let x = 42
x { // ← x is an int, not a block
}
{{< /err >}}

Trailing blocks only work on functions that return `block[T]`, like `std.deploy`.

## lang.CannotIndex

An index expression `x[i]` was used on a value that doesn't support indexing.

{{< err >}}
module main
let x = 42
let y = x[0] // ← can't index an int
{{< /err >}}

Only lists and maps support indexing.

## lang.DeclMissingReturnType

A `decl` or `func` declaration is missing its return type annotation.

{{< err >}}
module main
func double(x: int) {
  return x + x
}
{{< /err >}}

{{< fix >}}
module main
func double(x: int) int {
  return x + x
}
{{< /fix >}}

## lang.DuplicateField

The same field was assigned twice in a struct literal.

{{< err >}}
module main
import "std/posix"
posix.dir {
  path = "/srv/app"
  perm = "0755"
  path = "/srv/other" // ← already set above
}
{{< /err >}}

Remove the duplicate assignment. The LSP offers a "Remove duplicate field"
quick fix.

## lang.DuplicateImport

The same module was imported twice.

{{< err >}}
module main
import "std/posix"
import "std/posix" // ← duplicate
{{< /err >}}

Remove the duplicate line. The LSP offers a "Remove duplicate import" quick fix.

## lang.DuplicateLet

A `let` binding reuses a name that's already bound in the same scope.

{{< err >}}
module main
let x = 1
let x = 2 // ← already defined above
{{< /err >}}

Use a different name, or remove the first binding if it's no longer needed.

## lang.EnvVarNotSet

`std.env()` was called for an environment variable that isn't set, and no
default was provided.

{{< err >}}
module main
import "std"
let token = std.env("API_TOKEN") // ← fails if API_TOKEN is unset
{{< /err >}}

{{< fix >}}
module main
import "std"
let token = std.env("API_TOKEN", default = "")
{{< /fix >}}

Provide a `default` parameter to make the lookup non-fatal, or set the
variable before running scampi.

## lang.Error

Generic checker or evaluator error. This is the fallback code when no more
specific code applies. The error message has the details.

## lang.FieldTypeMismatch

A field value doesn't match the expected type.

{{< err >}}
module main
import "std/posix"
posix.copy {
  src  = posix.source_local { path = "./x" }
  dest = "/etc/x"
  perm = 644 // ← perm is string, not int (use "0644")
}
{{< /err >}}

{{< fix >}}
module main
import "std/posix"
posix.copy {
  src  = posix.source_local { path = "./x" }
  dest = "/etc/x"
  perm = "0644"
}
{{< /fix >}}

The error message shows the expected and actual types. Numeric file modes are
strings in scampi — this matches the `chmod` convention and avoids octal
ambiguity.

## lang.ForInRequiresList

A `for ... in` loop was given a value that isn't a list.

{{< err >}}
module main
func f() int {
  for x in 42 { // ← 42 is not a list
  }
  return 0
}
{{< /err >}}

{{< fix >}}
module main
func f() int {
  for x in [1, 2, 3] {
  }
  return 0
}
{{< /fix >}}

## lang.GenericArgCount

A generic type was instantiated with the wrong number of type arguments.

{{< err >}}
module main
type Foo {
  x: map[string] // ← map takes 2 type args: map[K, V]
}
{{< /err >}}

{{< fix >}}
module main
type Foo {
  x: map[string, int]
}
{{< /fix >}}

## lang.IfNotBool

An `if` condition must be a boolean expression.

{{< err >}}
module main
func f(x: int) int {
  if x {  // ← x is int, not bool
    return 1
  }
  return 0
}
{{< /err >}}

{{< fix >}}
module main
func f(x: int) int {
  if x > 0 {
    return 1
  }
  return 0
}
{{< /fix >}}

scampi has no truthy/falsy coercion. Use an explicit comparison.

## lang.IndeterminateType

The checker can't determine the type of an expression. This usually happens
with empty collection literals where the element type can't be inferred.

## lang.LetTypeMismatch

A `let` binding has an explicit type annotation that doesn't match the value.

{{< err >}}
module main
let x: int = "hello" // ← string doesn't match int
{{< /err >}}

Either fix the type annotation or the value.

## lang.ListIndexNotInt

A list index must be an integer.

{{< err >}}
module main
let xs = [1, 2, 3]
let y = xs["0"] // ← index must be int, not string
{{< /err >}}

{{< fix >}}
module main
let xs = [1, 2, 3]
let y = xs[0]
{{< /fix >}}

## lang.MapKeyMismatch

A map literal has keys of inconsistent types, or a map access uses the wrong
key type.

{{< err >}}
module main
let m = {1: "one", "two": 2} // ← int and string keys mixed
{{< /err >}}

All keys in a map literal must have the same type.

## lang.MarkerAttrArgs

A marker attribute (one with no fields) was given arguments.

{{< err >}}
module main
type Foo {
  @std.nonempty("extra") // ← nonempty takes no arguments
  x: string
}
{{< /err >}}

{{< fix >}}
module main
type Foo {
  @std.nonempty
  x: string
}
{{< /fix >}}

## lang.MissingField

A required field was not provided in a struct literal or step invocation.

{{< err >}}
module main
import "std/posix"
posix.copy {
  src = posix.source_local { path = "./x" }
  // ← dest is required but missing
}
{{< /err >}}

{{< fix >}}
module main
import "std/posix"
posix.copy {
  src  = posix.source_local { path = "./x" }
  dest = "/etc/app/x"
}
{{< /fix >}}

Required fields are those without a default value in the type definition. The
error message names the missing field.

## lang.ModuleMemberUndef

A member was accessed on a module that doesn't export it.

{{< err >}}
module main
import "std/posix"
posix.nonexistent {} // ← posix has no member called nonexistent
{{< /err >}}

Check the module's exports. The LSP offers completions after typing the dot.

## lang.MutationOutsideFunc

An assignment (`x[i] = v` or `x.field = v`) appeared outside a function body.
Mutations are only allowed inside `func` bodies.

{{< err >}}
module main
let xs = [1, 2, 3]
xs[0] = 99 // ← can't mutate at top level
{{< /err >}}

## lang.NoField

A field access `.name` was used on a type that has no field with that name.
Similar to `lang.UnknownField` but triggered during type resolution rather
than struct literal checking.

## lang.NoVariant

An enum variant was referenced that doesn't exist on the enum type.

{{< err >}}
module main
import "std/posix"
let s = posix.ServiceState.restarted // ← not a variant
{{< /err >}}

Check the enum definition for valid variants. The LSP offers completions
after the dot.

## lang.NotAModule

A dotted access `x.member` was used where `x` resolves to a value, not a
module. Module members use dot syntax, but so does field access — the checker
needs to distinguish them.

## lang.NotBlockType

A trailing block `{ ... }` was used on a type that isn't `block[T]`.

{{< err >}}
module main
import "std/posix"
let x = posix.source_local { path = "./x" }
x { // ← source_local doesn't return a block type
}
{{< /err >}}

## lang.NotOpNotBool

The `!` (not) operator was used on a non-boolean value.

{{< err >}}
module main
func f(x: int) bool {
  return !x // ← x is int, not bool
}
{{< /err >}}

{{< fix >}}
module main
func f(x: int) bool {
  return !(x > 0)
}
{{< /fix >}}

## lang.NotStructOrDecl

A struct literal `Type { ... }` was used with something that isn't a type
or decl.

{{< err >}}
module main
let x = 42
let y = x { name = "foo" } // ← x is not a type
{{< /err >}}

## lang.OpaqueConstruct

An opaque type (declared without a body) was used as a struct literal.
Opaque types can only be created by their defining module's functions.

{{< err >}}
module main
import "std"
let t = std.Target { name = "x" } // ← Target is opaque
{{< /err >}}

Use the appropriate constructor instead (e.g. `local.target`, `ssh.target`).

## lang.ReturnTypeMismatch

A function's `return` expression doesn't match the declared return type.

{{< err >}}
module main
func double(x: int) int {
  return "not a number" // ← string doesn't match int
}
{{< /err >}}

## lang.SecretLookup

A `std.secret()` call failed — either no secrets backend is configured, the
secrets file couldn't be read, or the key doesn't exist.

{{< err >}}
module main
import "std"
let key = std.secret("db.password") // ← no std.secrets {} configured yet
{{< /err >}}

{{< fix >}}
module main
import "std"
std.secrets {
  backend = std.SecretsBackend.age
  path    = "secrets.age.json"
}
let key = std.secret("db.password")
{{< /fix >}}

If the backend is configured but the key is missing, check your secrets file.

## lang.SelfOutsideStep

`self` was used outside a `decl` body. The `self` keyword is only available
inside `decl` declarations, where it refers to the call-site struct literal.

{{< err >}}
module main
func f() string {
  return self.name // ← self is not available in func
}
{{< /err >}}

## lang.TooFewArgs

A function was called with fewer positional arguments than required.

{{< err >}}
module main
import "std"
std.deploy() { // ← needs at least name and targets
}
{{< /err >}}

Check the function signature. The LSP shows parameter hints on hover and
offers signature help while typing.

## lang.TooManyArgs

A function was called with more positional arguments than it accepts.

{{< err >}}
module main
import "std"
import "std/local"
let host = local.target { name = "h" }
std.deploy("web", [host], "extra") { // ← too many args
}
{{< /err >}}

## lang.UnaryMinusNotInt

The unary `-` operator was used on a non-integer value.

{{< err >}}
module main
let x = -"hello" // ← can't negate a string
{{< /err >}}

## lang.Undefined

A name was used that doesn't exist in the current scope.

{{< err >}}
module main
import "std/posix"

posix.copy {
  src  = posix.source_local { path = "./app.conf" }
  dest = target_path // ← nothing called target_path in scope
}
{{< /err >}}

Check for typos. If the name is from another module, make sure you've imported
it. The LSP will offer an "Add import" quick fix when the name matches a known
standard library module.

## lang.UnknownAttribute

An attribute was used that doesn't exist in any imported module.

{{< err >}}
module main
type Foo {
  @std.nonexistent
  x: string
}
{{< /err >}}

Check the attribute name. Standard library attributes are listed in the
[Language reference]({{< relref "../language" >}}#attributes).

## lang.UnknownAttrFieldType

An attribute type declaration references a field type that doesn't exist.
This is an error in the attribute type definition itself, not in user code.

## lang.UnknownField

A field name was used that doesn't exist on the target type.

{{< err >}}
module main
import "std/posix"
posix.copy {
  src  = posix.source_local { path = "./x" }
  dest = "/etc/app/x"
  mode = "0644" // ← the field is called 'perm', not 'mode'
}
{{< /err >}}

Check the step reference for the correct field names. The LSP offers
completions inside struct literals.

## lang.UnknownFieldType

A field's type annotation references an unknown type.

{{< err >}}
module main
type Foo {
  x: Nonexistent
}
{{< /err >}}

Check the type name for typos, or import the module that defines it.

## lang.UnknownGenericType

A generic type name doesn't match any known generic type. The built-in
generic types are `list[T]`, `map[K, V]`, and `block[T]`.

{{< err >}}
module main
type Foo {
  x: set[string] // ← no generic type called 'set'
}
{{< /err >}}

## lang.UnknownModule

An imported module path doesn't match any known module.

{{< err >}}
module main
import "std/networking" // ← no such module
{{< /err >}}

The standard library modules are `std`, `std/posix`, `std/local`, `std/ssh`,
`std/container`, `std/rest`, and `std/test`. User modules are declared in
`scampi.mod`.

## lang.UnknownType

A type name was used that doesn't exist in the current scope.

{{< err >}}
module main
type Foo {
  x: Widget // ← no type called Widget
}
{{< /err >}}

Check for typos. If the type is from another module, qualify it with the
module name (e.g. `posix.ServiceState`).

