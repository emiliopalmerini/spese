function recurrentForm() {
  return {
    categories: [],
    selectedPrimary: '',
    selectedSecondary: '',
    selectedFrequency: '',
    loading: true,

    frequencies: [
      { value: 'daily', label: 'Giornaliera' },
      { value: 'weekly', label: 'Settimanale' },
      { value: 'monthly', label: 'Mensile' },
      { value: 'yearly', label: 'Annuale' }
    ],

    get currentSecondaries() {
      const cat = this.categories.find(c => c.primary === this.selectedPrimary);
      return cat ? cat.secondaries : [];
    },

    get isValid() {
      return this.selectedPrimary && this.selectedSecondary && this.selectedFrequency;
    },

    async init() {
      // Load categories
      try {
        const resp = await fetch('/api/categories');
        this.categories = await resp.json();
      } catch (e) {
        console.error('Failed to load categories:', e);
      }
      this.loading = false;

      // Focus amount input
      this.$nextTick(() => {
        this.$refs.amountInput?.focus();
      });

      // Pre-select first category and subcategory
      if (this.categories.length > 0) {
        this.selectedPrimary = this.categories[0].primary;
        if (this.currentSecondaries.length > 0) {
          this.selectedSecondary = this.currentSecondaries[0];
        }
      }
    },

    selectPrimary(primary) {
      this.selectedPrimary = primary;
      this.selectedSecondary = '';
      if (this.currentSecondaries.length === 1) {
        this.selectedSecondary = this.currentSecondaries[0];
      }
    },

    selectSecondary(secondary) {
      this.selectedSecondary = secondary;
    },

    selectFrequency(freq) {
      this.selectedFrequency = freq;
    },

    formatAmount(event) {
      let value = event.target.value;
      value = value.replace(/[^\d,\.]/g, '');
      value = value.replace(',', '.');
      event.target.value = value;
    }
  }
}
