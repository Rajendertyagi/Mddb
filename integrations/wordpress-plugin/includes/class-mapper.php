<?php
/**
 * Converts a WP_Post to the MDDB /v1/add payload.
 *
 * @package Tradik\MddbSync
 */

declare(strict_types=1);

namespace Tradik\MddbSync;

defined( 'ABSPATH' ) || exit;

/**
 * Pure mapping layer — no I/O, no hooks. Receives a `WP_Post` and emits
 * the JSON document we send to MDDB.
 */
final class Mapper {

	private Language $language;

	public function __construct( Language $language ) {
		$this->language = $language;
	}

	/**
	 * @param \WP_Post $post
	 * @return array{collection:string,key:string,lang:string,meta:array<string,array<int,string>>,contentMd:string}
	 */
	public function toDocument( \WP_Post $post, string $collection, string $keyStrategy, string $languageStrategy ): array {
		return [
			'collection' => $collection,
			'key'        => $this->keyFor( $post, $keyStrategy ),
			'lang'       => $this->language->detect( (int) $post->ID, $languageStrategy ),
			'meta'       => $this->metaFor( $post ),
			'contentMd'  => $this->bodyFor( $post ),
		];
	}

	public function keyFor( \WP_Post $post, string $strategy ): string {
		switch ( $strategy ) {
			case Settings::KEY_POST_SLUG:
				$slug = (string) $post->post_name;
				if ( $slug === '' ) {
					$slug = sanitize_title( (string) $post->post_title );
				}
				$prefix = sanitize_key( (string) $post->post_type );
				return $prefix . '-' . ( $slug !== '' ? $slug : (string) $post->ID );
			case Settings::KEY_PERMALINK:
				$permalink = get_permalink( $post );
				if ( is_string( $permalink ) && $permalink !== '' ) {
					$path = (string) wp_parse_url( $permalink, PHP_URL_PATH );
					$path = trim( $path, '/' );
					if ( $path !== '' ) {
						return $path;
					}
				}
				return sanitize_key( (string) $post->post_type ) . '-' . (string) $post->ID;
			case Settings::KEY_POST_ID:
			default:
				return sanitize_key( (string) $post->post_type ) . '-' . (string) $post->ID;
		}
	}

	/**
	 * @return array<string,array<int,string>>
	 */
	private function metaFor( \WP_Post $post ): array {
		$meta = [
			'postType'   => [ (string) $post->post_type ],
			'status'     => [ (string) $post->post_status ],
			'title'      => [ (string) $post->post_title ],
			'slug'       => [ (string) $post->post_name ],
			'permalink'  => [ (string) get_permalink( $post ) ],
			'authorId'   => [ (string) $post->post_author ],
			'publishedAt' => [ (string) get_post_time( 'c', true, $post ) ],
			'modifiedAt'  => [ (string) get_post_modified_time( 'c', true, $post ) ],
		];

		$authorName = get_the_author_meta( 'display_name', (int) $post->post_author );
		if ( is_string( $authorName ) && $authorName !== '' ) {
			$meta['author'] = [ $authorName ];
		}

		$excerpt = (string) $post->post_excerpt;
		if ( $excerpt !== '' ) {
			$meta['excerpt'] = [ $excerpt ];
		}

		$meta['categories'] = $this->terms( $post, 'category' );
		$meta['tags']       = $this->terms( $post, 'post_tag' );

		// Drop empty arrays so MDDB meta stays compact.
		return array_filter(
			$meta,
			static fn( array $values ): bool => count( $values ) > 0 && trim( (string) ( $values[0] ?? '' ) ) !== ''
		);
	}

	/**
	 * @return array<int,string>
	 */
	private function terms( \WP_Post $post, string $taxonomy ): array {
		$terms = get_the_terms( $post, $taxonomy );
		if ( ! is_array( $terms ) ) {
			return [];
		}
		$names = [];
		foreach ( $terms as $term ) {
			if ( $term instanceof \WP_Term && $term->name !== '' ) {
				$names[] = $term->name;
			}
		}
		return $names;
	}

	private function bodyFor( \WP_Post $post ): string {
		$title  = (string) $post->post_title;
		// phpcs:ignore WordPress.NamingConventions.PrefixAllGlobals.NonPrefixedHooknameFound -- core WP filter.
		$html   = (string) apply_filters( 'the_content', (string) $post->post_content );
		$plain  = wp_strip_all_tags( $html, true );
		$header = $title !== '' ? '# ' . $title . "\n\n" : '';
		return $header . trim( $plain ) . "\n";
	}
}
