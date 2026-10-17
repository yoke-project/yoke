# A descriptor's parameters

| | |
| --- | --- |
| **Feature** | what a site may supply and nothing else: groups named after a unit, a channel or `shared`; each declaration's type, its constraint and its label; exactly one arrangement of `required` and `default`; and the values a site supplies checked against the declarations — a value failing its declaration, and a required parameter with none, each refused by name |
| **Planning item** | yoke-project/yoke#228 |

## yoke:the-parameters.01 — declarations are read and handed on with the deployment

| Field | Value |
| --- | --- |
| **Cites** | specs/12.17 · specs/14.41 · arch/15-gate/04 §What only the descriptor has |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a descriptor declaring, in `shared` and in a unit's group, one parameter of each of the six types |
| **Action** | check it |
| **Expected** | nothing is refused, and the deployment carries each declaration with its type, its constraint, whether it is required, its default and its label |

## yoke:the-parameters.02 — a declaration is what its type allows

| Field | Value |
| --- | --- |
| **Cites** | specs/12.43 · specs/14.41 · arch/15-gate/04 §What only the descriptor has · arch/15-gate/06 §Phase 1 — shape |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | declarations with an unknown key, a type outside the six, a `range` on a string, a `pattern` on an integer, an `enum` with no values, a pattern that does not compile, a default its own declaration refuses, and no label |
| **Action** | check each |
| **Expected** | each is refused, naming the parameter and what is wrong with it |

## yoke:the-parameters.03 — exactly one arrangement of `required` and `default` is legal

| Field | Value |
| --- | --- |
| **Cites** | specs/12.43 · specs/14.41 · arch/15-gate/06 §Phase 1 — shape |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a parameter required with a default, and one neither required nor defaulted |
| **Action** | check each |
| **Expected** | each is refused `parameter.arrangement`, naming it |

## yoke:the-parameters.04 — a group is named after a unit, a channel or `shared`

| Field | Value |
| --- | --- |
| **Cites** | specs/17.23 · arch/15-gate/04 §What only the descriptor has |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | a descriptor declaring a group `panel` where `panel` is neither a unit nor a channel |
| **Action** | check it |
| **Expected** | it is refused, naming the group |

## yoke:the-parameters.05 — supplied values are checked against the declarations, and the defaults fill the rest

| Field | Value |
| --- | --- |
| **Cites** | specs/12.41 · specs/14.41 · specs/14.42 · specs/17.22 · specs/17.24 · arch/10-manager/07 §The values a site supplies · arch/15-gate/06 §Phase 4 — host facts |
| **Level** | L1 |
| **Method** | test |
| **Not applicable in** | — |
| **Label** | blocking |
| **Precondition** | declarations of each type, one required with no default; values that satisfy them, then values that each fail one declaration — a string the pattern refuses, an integer outside the range, a value no enum permits, a boolean that is not one, a path that does not exist — then none for the required parameter, then a value for a parameter nobody declared |
| **Action** | check each set of values |
| **Expected** | the first resolves to the values given and the defaults for the rest; each failing value is refused `parameter.value`, naming the parameter, the value and the declaration; the absent required value is refused `parameter.missing`; the undeclared one is refused, naming it |
