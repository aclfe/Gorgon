module.exports = {
    extends: ['@commitlint/config-conventional'],
    rules: {
        'type-enum': [0],
        'body-max-line-length': [0, 'always', Infinity],
        'header-format': [2, 'always'],
        'issue-reference': [2, 'always'],
    },
    plugins: [
        {
            rules: {
                'header-format': ({ header }) => {
                    const conventionalRegex = /^(build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test)(\(.*\))?: .+/;
                    const issueRegex = /^Issue #\d+: .+/;
                    if (conventionalRegex.test(header) || issueRegex.test(header)) {
                        return [true];
                    }
                    return [false, 'Header must start with a conventional type (e.g., "fix: ") or "Issue #123: "'];
                },
                'issue-reference': async ({ header, body }) => {
                    if (!body) {
                        return [false, 'Commit body is required'];
                    }

                    const issueInHeaderMatch = header.match(/^Issue #(\d+):/);
                    const issueNumInHeader = issueInHeaderMatch ? issueInHeaderMatch[1] : null;

                    if (issueNumInHeader) {
                        const expectedInBody = `Issue #${issueNumInHeader}`;
                        if (!body.includes(expectedInBody)) {
                            return [false, `Body must contain "${expectedInBody}" when header references Issue #${issueNumInHeader}`];
                        }
                        const response = await fetch(`https://api.github.com/repos/${process.env.GITHUB_REPOSITORY}/issues/${issueNumInHeader}`, {
                            headers: {
                                'Authorization': `token ${process.env.GITHUB_TOKEN}`,
                                'Accept': 'application/vnd.github.v3+json'
                            }
                        });
                        if (!response.ok) {
                            return [false, `Issue #${issueNumInHeader} does not exist in the repository`];
                        }
                        return [true];
                    } else {
                        const issueInBodyMatch = body.match(/Issue #(\d+|nil)/);
                        if (!issueInBodyMatch) {
                            return [false, 'Body must contain "Issue #nil" or "Issue #xxx" (with a valid issue number)'];
                        }
                        const issueNumInBody = issueInBodyMatch[1];
                        if (issueNumInBody !== 'nil') {
                            const response = await fetch(`https://api.github.com/repos/${process.env.GITHUB_REPOSITORY}/issues/${issueNumInBody}`, {
                                headers: {
                                    'Authorization': `token ${process.env.GITHUB_TOKEN}`,
                                    'Accept': 'application/vnd.github.v3+json'
                                }
                            });
                            if (!response.ok) {
                                return [false, `Issue #${issueNumInBody} does not exist in the repository`];
                            }
                        }
                        return [true];
                    }
                },
            },
        },
    ],
};