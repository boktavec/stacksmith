# Strict input parsing

StackSmith accepts one JSON or YAML document per input file. Both formats decode to the same Go types; parsing checks document structure, while `Validate` checks meaning and policy. A parse failure returns diagnostics and a zero value, so callers cannot accidentally use partially decoded input.

The standard `encoding/json` struct decoder is not strict enough alone: duplicate object keys are accepted, and field names match case-insensitively. StackSmith first walks decoder tokens to reject duplicate and unknown keys, wrong JSON types, nulls, and trailing values; it then decodes with `DisallowUnknownFields` as a second check.

YAML has its own implicit scalar conversions, aliases, merge keys, custom tags, and duplicate mappings. StackSmith first inspects the `yaml.Node` tree, accepting only JSON-compatible scalar and collection forms, checking duplicates and exact field names, and rejecting anchors/aliases, merge keys, custom tags, non-string keys, and multiple documents. `KnownFields(true)` remains enabled for the typed decode.

Both paths enforce the 1 MiB input limit before parsing. Structural failures stop at the first diagnostic, and a header-only pass checks `apiVersion` and `kind` first so a wrong version or kind is reported before unknown fields (structural errors such as null, anchors, aliases, merge keys, custom tags and duplicate keys, and wrong-case `apiVersion`/`kind` keys, are also reported by that first pass, before an unsupported-version diagnostic); semantic validation collects all independent problems in stable input order. These checks validate the local contract only: they do not contact AWS or prove that an account, region, blueprint name, or permission exists remotely.
