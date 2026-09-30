# Copyright (c) 2025, PostgreSQL Global Development Group

transaction_transients_trace_sources = files(
  'transaction_transients_trace.c',
)

if host_system == 'windows'
  transaction_transients_trace_sources += rc_lib_gen.process(win32ver_rc, extra_args: [
    '--NAME', 'transaction_transients_trace',
    '--FILEDESC', 'transaction_transients_trace - trace transient relation lifecycle (skeleton)',])
endif

transaction_transients_trace = shared_module('transaction_transients_trace',
  transaction_transients_trace_sources,
  kwargs: contrib_mod_args,
)
contrib_targets += transaction_transients_trace

tests += {
  'name': 'transaction_transients_trace',
  'sd': meson.current_source_dir(),
  'bd': meson.current_build_dir(),
  'regress': {
    'sql': [
      'transaction_transients_trace',
    ],
  },
}
