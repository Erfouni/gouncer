// SPDX-License-Identifier: Apache-2.0

/** The letters, digits and ten symbols a generated password draws from. */
const CHARACTERS = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()'

/** The first byte value that would make some characters likelier than others. */
const UNBIASED_BOUND = 256 - (256 % CHARACTERS.length)

/** The most bytes crypto.getRandomValues fills in one draw, so the longest password the generator makes. */
const LONGEST_PASSWORD = 65536

/**
 * Reports whether the generator makes a password this long.
 * @param length - The number of characters asked for.
 * @returns True for a whole number from 1 to 65536.
 */
function isAllowedLength(length: number): boolean {
	return Number.isInteger(length) && length >= 1 && length <= LONGEST_PASSWORD
}

/**
 * Returns a random password from the generator's characters, each equally likely.
 * @param length - The number of characters the password holds, a whole number from 1 to 65536.
 * @returns The password.
 */
export function generatePassword(length: number): string {
	if (!isAllowedLength(length)) {
		throw new RangeError(
			`generatedPasswordLength must be a whole number from 1 to ${LONGEST_PASSWORD}, not ${length}`,
		)
	}
	const bytes = new Uint8Array(length)
	let password = ''
	while (password.length < length) {
		crypto.getRandomValues(bytes)
		const kept = bytes.filter((byte) => byte < UNBIASED_BOUND)
		for (const byte of kept.slice(0, length - password.length)) {
			password += CHARACTERS[byte % CHARACTERS.length]
		}
	}
	return password
}
