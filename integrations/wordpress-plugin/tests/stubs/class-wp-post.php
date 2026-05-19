<?php
/**
 * Minimal WP_Post stand-in for unit tests.
 *
 * @package Tradik\MddbSync\Tests
 */

declare(strict_types=1);

/**
 * Public-property bag matching the fields our Mapper/Sync reads.
 */
class WP_Post { // phpcs:ignore
	public int $ID = 0;
	public string $post_title = '';      // phpcs:ignore
	public string $post_name = '';       // phpcs:ignore
	public string $post_content = '';    // phpcs:ignore
	public string $post_excerpt = '';    // phpcs:ignore
	public string $post_type = 'post';   // phpcs:ignore
	public string $post_status = 'publish'; // phpcs:ignore
	public int $post_author = 1;         // phpcs:ignore

	/**
	 * @param array<string,mixed> $fields
	 */
	public function __construct( array $fields = [] ) {
		foreach ( $fields as $key => $value ) {
			if ( property_exists( $this, $key ) ) {
				$this->{$key} = $value;
			}
		}
	}
}
