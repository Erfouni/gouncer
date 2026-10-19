// SPDX-License-Identifier: Apache-2.0

/** The letters, digits and ten symbols a generated password draws from. */
const CHARACTERS = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()'

/** The first byte value that would make some characters likelier than others. */
const UNBIASED_BOUND = 256 - (256 % CHARACTERS.length)

/**
 * Returns a random password from the generator's characters, each equally likely.
 * @param length - The number of characters the password holds.
 * @returns The password.
 */
export function generatePassword(length: number): string {
	const wanted = Math.trunc(length)
	const bytes = new Uint8Array(wanted)
	let password = ''
	while (password.length < wanted) {
		crypto.getRandomValues(bytes)
		const kept = bytes.filter((byte) => byte < UNBIASED_BOUND)
		for (const byte of kept.slice(0, wanted - password.length)) {
			password += CHARACTERS[byte % CHARACTERS.length]
		}
	}
	return password
}
