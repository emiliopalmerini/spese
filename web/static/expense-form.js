function expenseForm() {
  return {
    categories: [],
    selectedPrimary: '',
    selectedSecondary: '',
    selectedDate: '',
    loading: true,

    get currentSecondaries() {
      const cat = this.categories.find(c => c.primary === this.selectedPrimary);
      return cat ? cat.secondaries : [];
    },

    get selectedDay() {
      if (!this.selectedDate) return '';
      return new Date(this.selectedDate).getDate();
    },

    get selectedMonth() {
      if (!this.selectedDate) return '';
      return new Date(this.selectedDate).getMonth() + 1;
    },

    get isValid() {
      return this.selectedPrimary && this.selectedSecondary;
    },

    async init() {
      // Set today's date
      const today = new Date();
      this.selectedDate = today.toISOString().split('T')[0];

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

      // Restore last used category from localStorage
      const lastPrimary = localStorage.getItem('lastPrimary');
      const lastSecondary = localStorage.getItem('lastSecondary');
      if (lastPrimary) {
        this.selectedPrimary = lastPrimary;
        if (lastSecondary && this.currentSecondaries.includes(lastSecondary)) {
          this.selectedSecondary = lastSecondary;
        }
      }
    },

    selectPrimary(primary) {
      this.selectedPrimary = primary;
      this.selectedSecondary = '';
      // Auto-select if only one secondary
      if (this.currentSecondaries.length === 1) {
        this.selectedSecondary = this.currentSecondaries[0];
      }
    },

    selectSecondary(secondary) {
      this.selectedSecondary = secondary;
      // Save to localStorage
      localStorage.setItem('lastPrimary', this.selectedPrimary);
      localStorage.setItem('lastSecondary', this.selectedSecondary);
    },

    formatAmount(event) {
      let value = event.target.value;
      // Allow only numbers and comma/dot
      value = value.replace(/[^\d,\.]/g, '');
      // Replace comma with dot for backend
      value = value.replace(',', '.');
      event.target.value = value;
    }
  }
}
