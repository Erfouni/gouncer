// SPDX-License-Identifier: Apache-2.0

import { readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'
import { expect, test } from 'vitest'

const naming = /(?:\bfrom\s*|\bimport\s*\(?\s*|\brequire\s*\(\s*)['"]zxcvbn['"]/g

function filesUnder(folder: string): string[] {
	return readdirSync(folder, { recursive: true, withFileTypes: true })
		.filter((entry) => entry.isFile())
		.map((entry) => join(entry.parentPath, entry.name))
}

test('counts every way a module can name zxcvbn, and never the bare word', () => {
	const named = [
		"import zxcvbn from 'zxcvbn'",
		"import 'zxcvbn'",
		"const zxcvbn = require('zxcvbn')",
		"export { default } from 'zxcvbn'",
		"await import('zxcvbn')",
	]
	const unnamed = ['/** loadMeter loads zxcvbn on demand. */', "const name = 'zxcvbn'"]

	for (const source of named) {
		expect(source.match(naming), source).toHaveLength(1)
	}
	for (const source of unnamed) {
		expect(source.match(naming), source).toBeNull()
	}
})

test('loads zxcvbn only on demand, from one module', () => {
	const found = filesUnder('src').flatMap((file) =>
		[...readFileSync(file, 'utf8').matchAll(naming)].map((match) => ({ file, specifier: match[0] })),
	)

	expect(found).toHaveLength(1)
	expect(found[0].file).toBe(join('src', 'wpds', 'strength.ts'))
	expect(found[0].specifier).toMatch(/^import\(/)
})
