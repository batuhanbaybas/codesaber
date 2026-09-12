import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { tags as t } from '@lezer/highlight'

// JetBrains-dark / Cursor-inspired readable syntax colors on #1e1f22.
export const darkSyntax = syntaxHighlighting(
  HighlightStyle.define([
    { tag: t.comment, color: '#6a737d', fontStyle: 'italic' },
    {
      tag: [t.keyword, t.modifier, t.operatorKeyword],
      color: '#d3739c',
      fontWeight: 'bold',
    },
    { tag: [t.string, t.special(t.string)], color: '#e4b872' },
    { tag: [t.number, t.bool, t.null], color: '#7aa2f7' },
    {
      tag: [t.function(t.variableName), t.function(t.propertyName)],
      color: '#79c0ff',
    },
    {
      tag: [t.definition(t.variableName), t.definition(t.propertyName)],
      color: '#e6edf3',
    },
    {
      tag: [t.typeName, t.className, t.namespace],
      color: '#56d4dd',
      fontStyle: 'italic',
    },
    { tag: t.propertyName, color: '#aceebb' },
    { tag: t.operator, color: '#c9d1d9' },
    { tag: t.punctuation, color: '#8b949e' },
    { tag: t.variableName, color: '#e6edf3' },
    { tag: t.tagName, color: '#79c0ff' },
    { tag: t.attributeName, color: '#d2a8ff' },
    {
      tag: t.invalid,
      color: '#f97583',
      textDecoration: 'underline wavy #f97583',
    },
  ]),
)
