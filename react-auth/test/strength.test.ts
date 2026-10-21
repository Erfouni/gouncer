// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from 'node:fs'
import { expect, test } from 'vitest'

import { disallowedWords, levelOf, loadMeter } from '../src/wpds/strength'

const manifest = JSON.parse(readFileSync('package.json', 'utf8')) as { dependencies: Record<string, string> }

const strongPassword = 'K7#mQ2!vR9@xL4$wP8'
const nameAndEmailWords = ['Maria', 'Perez', 'maria', 'example']
const strongOnlyWithCharacter64 = 'a'.repeat(57) + 'K7#mQ2!'
const strongOnlyWithCharacter65 = 'a'.repeat(58) + 'K7#mQ2!'

test('scores each WordPress level with the estimator WordPress ships', async () => {
	const meter = await loadMeter()

	expect(meter('abcdefghijkl', [])).toBe(0)
	expect(meter('password1234', [])).toBe(1)
	expect(meter('perezmaria12', disallowedWords(['Maria Perez', 'maria@example.com']))).toBe(2)
	expect(meter('twelvechars1', disallowedWords(['Maria Perez', 'maria@example.com']))).toBe(3)
	expect(meter(strongPassword, disallowedWords(['Maria Perez', 'maria@example.com']))).toBe(4)
})

test('lowers the score for the words it is given', async () => {
	const meter = await loadMeter()

	expect(meter('myapp12345678', [])).toBe(2)
	expect(meter('myapp12345678', ['myapp'])).toBe(1)
	expect(meter('mariaperez2026', disallowedWords(['Maria Perez', 'maria@example.com']))).toBe(3)
	expect(meter('mariaperez2026', disallowedWords(['Maria Perez', 'mariaperez@example.com']))).toBe(1)
})

test('builds the word list as WordPress does', () => {
	expect(disallowedWords(['Maria Perez', 'maria@example.com'])).toEqual(nameAndEmailWords)
	expect(disallowedWords(['Maria Perez', 'Perez Maria', '', 'maria'])).toEqual(['Maria', 'Perez', 'maria'])
})

test("maps each score to WordPress's word and tone", () => {
	expect(levelOf(-1)).toEqual({ word: 'Password strength unknown', tone: 'bad' })
	expect(levelOf(0)).toEqual({ word: 'Very weak', tone: 'short' })
	expect(levelOf(1)).toEqual({ word: 'Very weak', tone: 'short' })
	expect(levelOf(2)).toEqual({ word: 'Weak', tone: 'bad' })
	expect(levelOf(3)).toEqual({ word: 'Medium', tone: 'good' })
	expect(levelOf(4)).toEqual({ word: 'Strong', tone: 'strong' })
})

test('scores only the first 64 characters', async () => {
	const { default: zxcvbn } = await import('zxcvbn')
	const meter = await loadMeter()
	const pasted = 'a'.repeat(64) + strongPassword

	expect(pasted).toHaveLength(82)
	expect(zxcvbn(pasted).score).toBe(4)
	expect(meter(pasted, [])).toBe(0)
	expect(strongOnlyWithCharacter64).toHaveLength(64)
	expect(meter(strongOnlyWithCharacter64, [])).toBe(4)
	expect(zxcvbn(strongOnlyWithCharacter65).score).toBe(4)
	expect(meter(strongOnlyWithCharacter65, [])).toBe(3)
})

test('pins zxcvbn at the version WordPress ships', () => {
	expect(manifest.dependencies.zxcvbn).toBe('4.4.1')
})
