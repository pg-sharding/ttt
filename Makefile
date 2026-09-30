# contrib/transaction_transients_trace/Makefile

MODULES = transaction_transients_trace
PGFILEDESC = "transaction_transients_trace - trace transient relation lifecycle (skeleton)"

REGRESS = transaction_transients_trace

ifdef USE_PGXS
PG_CONFIG = pg_config
PGXS := $(shell $(PG_CONFIG) --pgxs)
include $(PGXS)
else
subdir = contrib/transaction_transients_trace
top_builddir = ../..
include $(top_builddir)/src/Makefile.global
include $(top_srcdir)/contrib/contrib-global.mk
endif
