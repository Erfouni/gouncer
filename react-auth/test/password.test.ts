// SPDX-License-Identifier: Apache-2.0

import { afterEach, expect, test, vi } from 'vitest'

import { generatePassword } from '../src/wpds/password'

afterEach(() => {
	vi.restoreAllMocks()
})

function drawInTurn(bytes: number[]) {
	const queue = [...bytes]
	return vi.spyOn(crypto, 'getRandomValues').mockImplementation((array) => {
		const view = array as Uint8Array
		for (let index = 0; index < view.length; index += 1) {
			view[index] = queue.shift() ?? 0
		}
		return array
	})
}

test("makes a password of the length asked from WordPress's characters", () => {
	for (const length of [18, 24]) {
		const password = generatePassword(length)

		expect(password).toHaveLength(length)
		expect(password).toMatch(/^[A-Za-z0-9!@#$%^&*()]+$/)
	}
	expect(generatePassword(2.5)).toHaveLength(2)
})

test('draws from crypto.getRandomValues', () => {
	const draw = vi.spyOn(crypto, 'getRandomValues')

	generatePassword(18)

	expect(draw).toHaveBeenCalled()
})

test('skips a byte that would favour some characters', () => {
	drawInTurn([250, 216, 215, 5, 6])

	expect(generatePassword(3)).toBe(')fg')
})

test("cycles through WordPress's 72 characters in its order", () => {
	const characters = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()'
	drawInTurn(Array.from({ length: 216 }, (_, index) => index))

	expect(generatePassword(216)).toBe(characters.repeat(3))
})
