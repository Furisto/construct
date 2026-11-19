use construct_types::event::{is_internal_event, match_pattern};

#[test]
fn go_match_pattern_cases() {
    let cases = [
        ("wildcard matches task.created", "*", "task.created", true),
        ("wildcard matches message.chunk", "*", "message.chunk", true),
        ("wildcard matches tool.called", "*", "tool.called", true),
        (
            "task.* matches task.created",
            "task.*",
            "task.created",
            true,
        ),
        (
            "task.* matches task.updated",
            "task.*",
            "task.updated",
            true,
        ),
        (
            "task.* matches task.deleted",
            "task.*",
            "task.deleted",
            true,
        ),
        (
            "task.* does not match message.created",
            "task.*",
            "message.created",
            false,
        ),
        (
            "message.* matches message.chunk",
            "message.*",
            "message.chunk",
            true,
        ),
        (
            "*.created matches task.created",
            "*.created",
            "task.created",
            true,
        ),
        (
            "*.created matches agent.created",
            "*.created",
            "agent.created",
            true,
        ),
        (
            "*.created does not match task.updated",
            "*.created",
            "task.updated",
            false,
        ),
        (
            "exact match task.created",
            "task.created",
            "task.created",
            true,
        ),
        (
            "exact match message.chunk",
            "message.chunk",
            "message.chunk",
            true,
        ),
        ("exact no match", "task.created", "task.updated", false),
        ("empty pattern", "", "task.created", false),
        ("single part pattern", "task", "task.created", false),
        ("single part event", "task.*", "task", false),
        (
            "wildcard does not match internal.task.trigger",
            "*",
            "internal.task.trigger",
            false,
        ),
        (
            "wildcard does not match internal.task.suspend",
            "*",
            "internal.task.suspend",
            false,
        ),
        (
            "*.trigger does not match internal.task.trigger",
            "*.trigger",
            "internal.task.trigger",
            false,
        ),
        (
            "internal.task.trigger matches itself",
            "internal.task.trigger",
            "internal.task.trigger",
            true,
        ),
        (
            "internal.task.suspend matches itself",
            "internal.task.suspend",
            "internal.task.suspend",
            true,
        ),
        (
            "internal.* matches internal.task.trigger",
            "internal.*",
            "internal.task.trigger",
            true,
        ),
        (
            "internal.* matches internal.task.suspend",
            "internal.*",
            "internal.task.suspend",
            true,
        ),
        (
            "wildcard still matches task.created",
            "*",
            "task.created",
            true,
        ),
        (
            "task.* still matches task.created",
            "task.*",
            "task.created",
            true,
        ),
    ];
    for (name, pattern, event, want) in cases {
        assert_eq!(match_pattern(pattern, event), want, "{name}");
    }
}
#[test]
fn internal_and_edge_cases() {
    for (value, want) in [
        ("", false),
        ("internal", false),
        ("internal.", true),
        ("internal.task.trigger", true),
        ("Internal.task", false),
        ("task.internal", false),
    ] {
        assert_eq!(is_internal_event(value), want);
    }
    for (pattern, event, want) in [
        ("", "", true),
        ("*", "", true),
        ("*.*", "task.created", false),
        ("internal.*", "internal.task.trigger", true),
        ("*.task.trigger", "internal.task.trigger", false),
        ("*.created", "task.sub.created", false),
    ] {
        assert_eq!(match_pattern(pattern, event), want);
    }
}
