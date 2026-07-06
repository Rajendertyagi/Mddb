<?php
/**
 * Settings page + "Test connection" / "Sync everything" AJAX endpoints.
 *
 * @package Tradik\MddbSync
 */

declare(strict_types=1);

namespace Tradik\MddbSync;

defined( 'ABSPATH' ) || exit;

/**
 * Renders the Settings → MDDB Sync screen and handles the test-connection
 * and bulk-resync AJAX endpoints.
 */
final class Admin {

	private const MENU_SLUG = 'mddb-sync';

	private const NONCE_TEST_CONNECTION = 'mddb_sync_test_connection';

	private const NONCE_BULK_RESYNC = 'mddb_sync_bulk_resync';

	private Settings $settings;

	private Client $client;

	private Bulk $bulk;

	public function __construct( Settings $settings, Client $client, Bulk $bulk ) {
		$this->settings = $settings;
		$this->client   = $client;
		$this->bulk     = $bulk;
	}

	public function register(): void {
		add_action( 'admin_menu', [ $this, 'addMenu' ] );
		add_action( 'admin_init', [ $this, 'registerSettings' ] );
		add_action( 'wp_ajax_mddb_sync_test_connection', [ $this, 'ajaxTestConnection' ] );
		add_action( 'wp_ajax_mddb_sync_bulk_resync', [ $this, 'ajaxBulkResync' ] );
		add_filter(
			'plugin_action_links_' . MDDB_SYNC_PLUGIN_BASENAME,
			[ $this, 'addSettingsLink' ]
		);
	}

	public function addMenu(): void {
		add_options_page(
			__( 'MDDB Sync', 'mddb-sync' ),
			__( 'MDDB Sync', 'mddb-sync' ),
			'manage_options',
			self::MENU_SLUG,
			[ $this, 'renderPage' ]
		);
	}

	public function registerSettings(): void {
		register_setting(
			'mddb_sync',
			Settings::OPTION_NAME,
			[
				'type'              => 'array',
				'sanitize_callback' => [ Settings::class, 'sanitize' ],
				'default'           => Settings::defaults(),
			]
		);
	}

	/**
	 * @param array<int,string> $links
	 * @return array<int,string>
	 */
	public function addSettingsLink( array $links ): array {
		$url            = admin_url( 'options-general.php?page=' . self::MENU_SLUG );
		$settingsLabel  = __( 'Settings', 'mddb-sync' );
		$settings_link  = '<a href="' . esc_url( $url ) . '">' . esc_html( $settingsLabel ) . '</a>';
		array_unshift( $links, $settings_link );
		return $links;
	}

	public function renderPage(): void {
		if ( ! current_user_can( 'manage_options' ) ) {
			return;
		}

		$values    = $this->settings->all();
		$postTypes = get_post_types( [ 'public' => true ], 'objects' );
		$termFilter = $this->settings->termFilter();
		?>
		<div class="wrap">
			<h1><?php esc_html_e( 'MDDB Sync', 'mddb-sync' ); ?></h1>
			<form method="post" action="options.php">
				<?php settings_fields( 'mddb_sync' ); ?>
				<?php // No presentation role: the row headers (<th scope="row">) convey real label→field relationships (Web:S5258). ?>
				<table class="form-table">
					<tr>
						<th scope="row"><label for="mddb_sync_url"><?php esc_html_e( 'MDDB URL', 'mddb-sync' ); ?></label></th>
						<td>
							<input type="url" id="mddb_sync_url" class="regular-text"
								name="<?php echo esc_attr( Settings::OPTION_NAME ); ?>[url]"
								value="<?php echo esc_attr( (string) $values['url'] ); ?>"
								placeholder="https://mddb.example.com" />
							<p class="description"><?php esc_html_e( 'Base URL of your MDDB server (no trailing slash).', 'mddb-sync' ); ?></p>
						</td>
					</tr>
					<tr>
						<th scope="row"><label for="mddb_sync_api_key"><?php esc_html_e( 'API key', 'mddb-sync' ); ?></label></th>
						<td>
							<input type="password" id="mddb_sync_api_key" class="regular-text"
								name="<?php echo esc_attr( Settings::OPTION_NAME ); ?>[apiKey]"
								value="<?php echo esc_attr( (string) $values['apiKey'] ); ?>"
								autocomplete="off" />
							<p class="description"><?php esc_html_e( 'Sent as Authorization: Bearer header. Leave empty for an unauthenticated dev instance.', 'mddb-sync' ); ?></p>
						</td>
					</tr>
					<tr>
						<th scope="row"><label for="mddb_sync_collection"><?php esc_html_e( 'Collection', 'mddb-sync' ); ?></label></th>
						<td>
							<input type="text" id="mddb_sync_collection" class="regular-text"
								name="<?php echo esc_attr( Settings::OPTION_NAME ); ?>[collection]"
								value="<?php echo esc_attr( (string) $values['collection'] ); ?>" />
							<p class="description"><?php esc_html_e( 'MDDB collection that receives every synced post. Defaults to the site host.', 'mddb-sync' ); ?></p>
						</td>
					</tr>
					<tr>
						<th scope="row"><?php esc_html_e( 'Sync events', 'mddb-sync' ); ?></th>
						<td>
							<label>
								<input type="checkbox" name="<?php echo esc_attr( Settings::OPTION_NAME ); ?>[syncOnSave]" value="1" <?php checked( ! empty( $values['syncOnSave'] ) ); ?> />
								<?php esc_html_e( 'On save / publish (POST /v1/add)', 'mddb-sync' ); ?>
							</label><br />
							<label>
								<input type="checkbox" name="<?php echo esc_attr( Settings::OPTION_NAME ); ?>[syncOnDelete]" value="1" <?php checked( ! empty( $values['syncOnDelete'] ) ); ?> />
								<?php esc_html_e( 'Clean entries on trash / delete (POST /v1/delete)', 'mddb-sync' ); ?>
							</label><br />
							<label>
								<input type="checkbox" name="<?php echo esc_attr( Settings::OPTION_NAME ); ?>[includeDrafts]" value="1" <?php checked( ! empty( $values['includeDrafts'] ) ); ?> />
								<?php esc_html_e( 'Include drafts (otherwise only publish status is synced)', 'mddb-sync' ); ?>
							</label>
						</td>
					</tr>
					<tr>
						<th scope="row"><?php esc_html_e( 'Post types', 'mddb-sync' ); ?></th>
						<td>
							<?php foreach ( $postTypes as $type ) : ?>
								<?php if ( $type->name === 'attachment' ) { continue; } ?>
								<label style="display:inline-block; margin-right:1.5em;">
									<input type="checkbox"
										name="<?php echo esc_attr( Settings::OPTION_NAME ); ?>[postTypes][]"
										value="<?php echo esc_attr( $type->name ); ?>"
										<?php checked( in_array( $type->name, (array) $values['postTypes'], true ) ); ?> />
									<?php echo esc_html( $type->labels->singular_name ); ?>
									<code><?php echo esc_html( $type->name ); ?></code>
								</label>
							<?php endforeach; ?>
						</td>
					</tr>
					<tr>
						<th scope="row"><label for="mddb_sync_language_source"><?php esc_html_e( 'Language detection', 'mddb-sync' ); ?></label></th>
						<td>
							<select id="mddb_sync_language_source" name="<?php echo esc_attr( Settings::OPTION_NAME ); ?>[languageSource]">
								<option value="<?php echo esc_attr( Settings::LANG_AUTO ); ?>" <?php selected( $values['languageSource'], Settings::LANG_AUTO ); ?>><?php esc_html_e( 'Auto (Polylang → WPML → site locale)', 'mddb-sync' ); ?></option>
								<option value="<?php echo esc_attr( Settings::LANG_POLYLANG ); ?>" <?php selected( $values['languageSource'], Settings::LANG_POLYLANG ); ?>><?php esc_html_e( 'Polylang', 'mddb-sync' ); ?></option>
								<option value="<?php echo esc_attr( Settings::LANG_WPML ); ?>" <?php selected( $values['languageSource'], Settings::LANG_WPML ); ?>><?php esc_html_e( 'WPML', 'mddb-sync' ); ?></option>
								<option value="<?php echo esc_attr( Settings::LANG_LOCALE ); ?>" <?php selected( $values['languageSource'], Settings::LANG_LOCALE ); ?>><?php esc_html_e( 'Site locale', 'mddb-sync' ); ?></option>
							</select>
						</td>
					</tr>
					<tr>
						<th scope="row"><label for="mddb_sync_key_strategy"><?php esc_html_e( 'Key strategy', 'mddb-sync' ); ?></label></th>
						<td>
							<select id="mddb_sync_key_strategy" name="<?php echo esc_attr( Settings::OPTION_NAME ); ?>[keyStrategy]">
								<option value="<?php echo esc_attr( Settings::KEY_POST_ID ); ?>" <?php selected( $values['keyStrategy'], Settings::KEY_POST_ID ); ?>><?php esc_html_e( 'Post type + ID  (post-123)', 'mddb-sync' ); ?></option>
								<option value="<?php echo esc_attr( Settings::KEY_POST_SLUG ); ?>" <?php selected( $values['keyStrategy'], Settings::KEY_POST_SLUG ); ?>><?php esc_html_e( 'Post type + slug (post-hello-world)', 'mddb-sync' ); ?></option>
								<option value="<?php echo esc_attr( Settings::KEY_PERMALINK ); ?>" <?php selected( $values['keyStrategy'], Settings::KEY_PERMALINK ); ?>><?php esc_html_e( 'Permalink path (2026/05/hello-world)', 'mddb-sync' ); ?></option>
							</select>
						</td>
					</tr>
					<tr>
						<th scope="row"><?php esc_html_e( 'Remote publishing (MCP)', 'mddb-sync' ); ?></th>
						<td>
							<label>
								<input type="checkbox" name="<?php echo esc_attr( Settings::OPTION_NAME ); ?>[enablePublish]" value="1" <?php checked( ! empty( $values['enablePublish'] ) ); ?> />
								<?php esc_html_e( 'Allow MDDB MCP tools to publish posts/pages on this site (POST /wp-json/mddb-sync/v1/publish and /status)', 'mddb-sync' ); ?>
							</label>
							<p class="description">
								<?php esc_html_e( 'Off by default. When enabled, every request must present the publish key below as an Authorization: Bearer header. Only the post types ticked above can be published.', 'mddb-sync' ); ?>
							</p>
						</td>
					</tr>
					<tr>
						<th scope="row"><label for="mddb_sync_publish_key"><?php esc_html_e( 'Publish key', 'mddb-sync' ); ?></label></th>
						<td>
							<input type="password" id="mddb_sync_publish_key" class="regular-text"
								name="<?php echo esc_attr( Settings::OPTION_NAME ); ?>[publishKey]"
								value="<?php echo esc_attr( (string) ( $values['publishKey'] ?? '' ) ); ?>"
								autocomplete="off" />
							<p class="description">
								<?php esc_html_e( 'Shared secret for inbound publishing. Leave empty and save with the toggle on to auto-generate a strong key. Configure the same key in MDDB via set_collection_config → wordpress {url, api_key}.', 'mddb-sync' ); ?>
							</p>
						</td>
					</tr>
					<tr>
						<th scope="row"><?php esc_html_e( 'Term filter', 'mddb-sync' ); ?></th>
						<td>
							<p class="description" style="margin-bottom:0.5em;">
								<?php esc_html_e( 'Tick terms to scope what gets synced. An empty list for a taxonomy = no filter. Across taxonomies: AND. Inside one taxonomy: OR.', 'mddb-sync' ); ?>
							</p>
							<?php $this->renderTermFilters( $termFilter ); ?>
						</td>
					</tr>
				</table>
				<?php submit_button(); ?>
			</form>

			<hr />
			<h2><?php esc_html_e( 'Test connection', 'mddb-sync' ); ?></h2>
			<p>
				<button type="button" class="button" id="mddb-sync-test"
					data-nonce="<?php echo esc_attr( wp_create_nonce( self::NONCE_TEST_CONNECTION ) ); ?>">
					<?php esc_html_e( 'Probe /v1/search', 'mddb-sync' ); ?>
				</button>
				<span id="mddb-sync-test-result" style="margin-left:0.5em;"></span>
			</p>

			<hr />
			<h2><?php esc_html_e( 'Sync everything', 'mddb-sync' ); ?></h2>
			<p class="description">
				<?php esc_html_e( 'Walks every published post of the selected types (and matching the term filter above) and re-sends it to MDDB. Run after changing the collection name, post-type list, or term filter — or after a fresh install.', 'mddb-sync' ); ?>
			</p>
			<p>
				<button type="button" class="button button-primary" id="mddb-sync-resync"
					data-nonce="<?php echo esc_attr( wp_create_nonce( self::NONCE_BULK_RESYNC ) ); ?>">
					<?php esc_html_e( 'Sync everything now', 'mddb-sync' ); ?>
				</button>
				<button type="button" class="button" id="mddb-sync-resync-stop" style="display:none;">
					<?php esc_html_e( 'Stop', 'mddb-sync' ); ?>
				</button>
			</p>
			<div id="mddb-sync-resync-status" style="margin-top:0.5em;"></div>
			<progress id="mddb-sync-resync-progress" value="0" max="100" style="width:400px; display:none;"></progress>

			<script>
			( function() {
				var testBtn   = document.getElementById( 'mddb-sync-test' );
				var resyncBtn = document.getElementById( 'mddb-sync-resync' );
				var stopBtn   = document.getElementById( 'mddb-sync-resync-stop' );
				var status    = document.getElementById( 'mddb-sync-resync-status' );
				var bar       = document.getElementById( 'mddb-sync-resync-progress' );

				if ( testBtn ) {
					testBtn.addEventListener( 'click', function() {
						var out = document.getElementById( 'mddb-sync-test-result' );
						out.textContent = '…';
						var body = new FormData();
						body.append( 'action', 'mddb_sync_test_connection' );
						body.append( '_ajax_nonce', testBtn.dataset.nonce );
						fetch( ajaxurl, { method: 'POST', credentials: 'same-origin', body: body } )
							.then( function( r ) { return r.json(); } )
							.then( function( j ) {
								out.textContent = ( j && j.data && j.data.message ) ? j.data.message : 'Unknown response.';
								out.style.color = ( j && j.success ) ? 'green' : 'red';
							} )
							.catch( function( e ) { out.textContent = String( e ); out.style.color = 'red'; } );
					} );
				}

				if ( ! resyncBtn ) { return; }
				var aborted = false;
				stopBtn.addEventListener( 'click', function() { aborted = true; } );

				resyncBtn.addEventListener( 'click', function() {
					aborted = false;
					resyncBtn.disabled = true;
					stopBtn.style.display = '';
					bar.style.display = '';
					bar.value = 0; bar.max = 100;
					status.textContent = '…';
					status.style.color = '';

					var totals = { succeeded: 0, failed: 0, skipped: 0, total: 0, errors: [] };

					function tick( offset ) {
						if ( aborted ) {
							finish( 'Stopped.' );
							return;
						}
						var body = new FormData();
						body.append( 'action', 'mddb_sync_bulk_resync' );
						body.append( '_ajax_nonce', resyncBtn.dataset.nonce );
						body.append( 'offset', String( offset ) );
						fetch( ajaxurl, { method: 'POST', credentials: 'same-origin', body: body } )
							.then( function( r ) { return r.json(); } )
							.then( function( j ) {
								if ( ! j || ! j.success ) {
									finish( ( j && j.data && j.data.message ) ? ( 'Error: ' + j.data.message ) : 'Error.', true );
									return;
								}
								var d = j.data;
								totals.succeeded += d.succeeded;
								totals.failed    += d.failed;
								totals.skipped   += d.skipped;
								totals.total      = d.total;
								if ( d.errors && d.errors.length ) { totals.errors = totals.errors.concat( d.errors ); }
								if ( d.total > 0 ) {
									bar.value = Math.min( 100, ( d.nextOffset / d.total ) * 100 );
								}
								status.textContent = (
									'Processed ' + ( d.nextOffset ) + ' / ' + d.total +
									' — ok: ' + totals.succeeded + ', skipped: ' + totals.skipped + ', failed: ' + totals.failed
								);
								if ( d.done ) { finish( 'Done.' ); return; }
								tick( d.nextOffset );
							} )
							.catch( function( e ) { finish( String( e ), true ); } );
					}

					function finish( msg, isError ) {
						resyncBtn.disabled = false;
						stopBtn.style.display = 'none';
						status.textContent = msg + ' ok: ' + totals.succeeded + ', skipped: ' + totals.skipped + ', failed: ' + totals.failed + '.';
						status.style.color = isError ? 'red' : ( totals.failed > 0 ? 'darkorange' : 'green' );
						if ( totals.errors.length ) {
							var pre = document.createElement( 'pre' );
							pre.style.cssText = 'background:#f6f7f7;padding:0.5em;border-left:4px solid #d63638;max-width:800px;overflow:auto;';
							pre.textContent = totals.errors.slice( 0, 10 ).join( '\n' );
							status.appendChild( pre );
						}
					}

					tick( 0 );
				} );
			} )();
			</script>
		</div>
		<?php
	}

	/**
	 * @param array<string,array<int,int>> $termFilter
	 */
	private function renderTermFilters( array $termFilter ): void {
		$postTypes = $this->settings->postTypes();
		$shown     = [];
		foreach ( $postTypes as $postType ) {
			$taxonomies = function_exists( 'get_object_taxonomies' )
				? (array) get_object_taxonomies( $postType, 'objects' )
				: [];
			foreach ( $taxonomies as $taxonomy ) {
				if ( ! $taxonomy instanceof \WP_Taxonomy ) {
					continue;
				}
				$slug = $taxonomy->name;
				if ( $slug === '' || ! $taxonomy->show_ui || isset( $shown[ $slug ] ) ) {
					continue;
				}
				$shown[ $slug ] = true;

				$selected = isset( $termFilter[ $slug ] ) ? array_map( 'intval', $termFilter[ $slug ] ) : [];
				$terms    = function_exists( 'get_terms' )
					? get_terms( [ 'taxonomy' => $slug, 'hide_empty' => false, 'number' => 200 ] )
					: [];
				if ( is_wp_error( $terms ) || ! is_array( $terms ) || count( $terms ) === 0 ) {
					continue;
				}
				?>
				<details open style="margin:0.75em 0; padding:0.5em 0.75em; border:1px solid #c3c4c7; background:#fff;">
					<summary style="font-weight:600;">
						<?php echo esc_html( $taxonomy->labels->name ); ?>
						<code><?php echo esc_html( $slug ); ?></code>
						<span style="font-weight:normal;color:#646970;">
							(<?php echo (int) count( $selected ); ?> / <?php echo (int) count( $terms ); ?> selected)
						</span>
					</summary>
					<div style="columns:3; column-gap:1.5em; margin-top:0.5em;">
						<?php foreach ( $terms as $term ) : ?>
							<?php if ( ! $term instanceof \WP_Term ) { continue; } ?>
							<label style="display:block; break-inside:avoid;">
								<input type="checkbox"
									name="<?php echo esc_attr( Settings::OPTION_NAME ); ?>[termFilter][<?php echo esc_attr( $slug ); ?>][]"
									value="<?php echo (int) $term->term_id; ?>"
									<?php checked( in_array( (int) $term->term_id, $selected, true ) ); ?> />
								<?php echo esc_html( $term->name ); ?>
								<span style="color:#646970;">(<?php echo (int) $term->count; ?>)</span>
							</label>
						<?php endforeach; ?>
					</div>
				</details>
				<?php
			}
		}
		if ( count( $shown ) === 0 ) {
			echo '<p><em>' . esc_html__( 'No taxonomies available for the selected post types — save the form first if you just changed post types.', 'mddb-sync' ) . '</em></p>';
		}
	}

	public function ajaxTestConnection(): void {
		if ( ! current_user_can( 'manage_options' ) ) {
			wp_send_json_error( [ 'message' => __( 'Forbidden.', 'mddb-sync' ) ], 403 );
		}
		check_ajax_referer( self::NONCE_TEST_CONNECTION );

		$result = $this->client->ping();
		if ( ! empty( $result['ok'] ) ) {
			wp_send_json_success( [ 'message' => $result['message'] ] );
		}
		wp_send_json_error( [ 'message' => $result['message'] ] );
	}

	public function ajaxBulkResync(): void {
		if ( ! current_user_can( 'manage_options' ) ) {
			wp_send_json_error( [ 'message' => __( 'Forbidden.', 'mddb-sync' ) ], 403 );
		}
		check_ajax_referer( self::NONCE_BULK_RESYNC );

		$offset = isset( $_POST['offset'] ) ? max( 0, (int) $_POST['offset'] ) : 0;
		$batch  = $this->bulk->processBatch( $offset, Bulk::DEFAULT_BATCH_SIZE );
		wp_send_json_success( $batch );
	}
}
