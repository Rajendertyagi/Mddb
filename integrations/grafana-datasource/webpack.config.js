// Webpack config for the MDDB Grafana datasource plugin.
// Bundles src/module.ts as an AMD module under the plugin id directory,
// copies plugin.json + static assets, and externalises Grafana runtime libs.

const path = require('path');
const CopyPlugin = require('copy-webpack-plugin');

const PLUGIN_ID = 'tradik-mddb-datasource';

module.exports = (_env, argv) => {
  const isProd = argv.mode === 'production';
  return {
    target: 'web',
    context: path.resolve(__dirname, 'src'),
    entry: { module: './module.ts' },
    mode: isProd ? 'production' : 'development',
    devtool: isProd ? 'source-map' : 'eval-source-map',
    output: {
      path: path.resolve(__dirname, 'dist'),
      filename: '[name].js',
      libraryTarget: 'amd',
      clean: true,
      publicPath: `public/plugins/${PLUGIN_ID}/`,
    },
    resolve: {
      extensions: ['.ts', '.tsx', '.js', '.jsx'],
    },
    module: {
      rules: [
        {
          test: /\.(t|j)sx?$/,
          loader: 'ts-loader',
          exclude: /node_modules/,
          options: { transpileOnly: true },
        },
        {
          test: /\.svg$/,
          type: 'asset/resource',
          generator: { filename: 'img/[name][ext]' },
        },
      ],
    },
    externals: ['react', 'react-dom', 'tslib', /^@grafana\//],
    plugins: [
      new CopyPlugin({
        patterns: [
          { from: 'plugin.json', to: '.' },
          { from: 'img', to: 'img' },
          { from: '../README.md', to: '.' },
          { from: '../CHANGELOG.md', to: '.' },
        ],
      }),
    ],
    optimization: {
      minimize: isProd,
    },
    performance: { hints: false },
  };
};
