(function(name, args, task, agent, directory) {
    const integer = x => {
        const n = Number(x);
        if (Number.isNaN(n) || n === 0) return 0;
        if (n >= 9223372036854775808) return '9223372036854775807';
        if (n <= -9223372036854775808) return '-9223372036854775808';
        const truncated = Math.trunc(n);
        return Math.abs(truncated) >= 9007199254740992 ? truncated.toFixed(0) : truncated;
    };
    const object = x => {
        if (x === null || x === undefined) throw new TypeError('Cannot convert undefined or null to object');
        return Object(x);
    };
    const a = args[0];
    if (name === 'create_file' || name === 'list_files') {
        const second = name === 'create_file' ? 'content' : 'recursive';
        let path, value;
        if (args.length === 1) {
            const o = object(a); path = o.path; value = o[second];
        } else if (args.length === 2) {
            path = a; value = args[1];
        }
        if (typeof path !== 'string' || typeof value !== (second === 'content' ? 'string' : 'boolean'))
            return {invalid: name};
        return {Path: path, [second === 'content' ? 'Content' : 'Recursive']: value};
    }
    if (!args.length) {
        if (name === 'edit_file') return {invalid: name};
        if (name === 'submit_report') return {invalid: name};
        return null;
    }
    switch (name) {
    case 'read_file': {
        const input = {path: String(a)};
        if (args.length >= 2 && args[1] != null) input.start_line = integer(args[1]);
        if (args.length >= 3 && args[2] != null) input.end_line = integer(args[2]);
        return input;
    }
    case 'execute_command': return {Command: String(a), WorkingDirectory: directory};
    case 'handoff': return {TaskID: task, CurrentAgentID: agent, RequestedAgent: String(a), HandoverMessage: args.length > 1 && args[1] !== undefined ? String(args[1]) : ''};
    case 'edit_file': {
        if (args.length < 2) return {invalid: name};
        const diffs = object(args[1]), result = [];
        for (let i = 0; i < integer(diffs.length); i++) {
            if (!(i in diffs)) continue;
            const d = object(diffs[i]);
            result.push({old: 'old' in d ? String(d.old) : '', new: 'new' in d ? String(d.new) : ''});
        }
        return {path: String(a), diffs: result.length ? result : null};
    }
    case 'grep':
    case 'find_file': {
        const o = object(a), input = {};
        for (const key of name === 'grep' ? ['query', 'path', 'include_pattern', 'exclude_pattern'] : ['pattern', 'path', 'exclude_pattern']) {
            input[key] = key in o ? String(o[key]) : '';
        }
        if (name === 'grep') input.case_sensitive = Boolean(o.case_sensitive);
        input.max_results = integer(o.max_results) || 50;
        return input;
    }
    case 'fetch': {
        const o = object(a), input = {url: '', timeout: 0};
        if (o.url !== undefined) input.url = String(o.url);
        if (o.timeout !== undefined) input.timeout = integer(o.timeout);
        if (o.headers !== undefined) {
            const h = object(o.headers); input.headers = {};
            for (const key of Object.keys(h)) if (h[key] !== undefined) input.headers[key] = String(h[key]);
        }
        return input;
    }
    case 'submit_report': {
        const o = object(a), input = {Summary: '', Completed: false, Deliverables: null, NextSteps: ''};
        if (o.summary !== undefined) input.Summary = String(o.summary);
        if (o.next_steps !== undefined) input.NextSteps = String(o.next_steps);
        if (o.completed !== undefined) input.Completed = Boolean(o.completed);
        if (o.deliverables !== undefined) {
            const d = object(o.deliverables);
            if (Array.isArray(d)) for (const v of d) if (v !== undefined) (input.Deliverables ??= []).push(String(v));
        }
        return input;
    }
    }
})
